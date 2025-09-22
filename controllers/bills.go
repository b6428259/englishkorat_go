package controllers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"englishkorat_go/database"
	"englishkorat_go/models"

	"github.com/gofiber/fiber/v2"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// BillsImportController handles importing bills/transactions from Wave export
type BillsImportController struct{}

// POST /api/import/bills
// Multipart form with file field: file
func (bc *BillsImportController) Import(c *fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is required"})
	}

	// Read rows
	var rows [][]string
	filename := strings.ToLower(fh.Filename)
	if strings.HasSuffix(filename, ".csv") {
		f, err := fh.Open()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot open file"})
		}
		defer f.Close()
		rows, err = readCSVSimple(f)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	} else if strings.HasSuffix(filename, ".xlsx") || strings.HasSuffix(filename, ".xls") {
		// buffer to temp path for excelize
		tmpDir, _ := os.MkdirTemp("", "ek-bills-")
		tmp := filepath.Join(tmpDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFilename(fh.Filename)))
		if err := c.SaveFile(fh, tmp); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to buffer upload"})
		}
		var rerr error
		rows, rerr = readXLSXSimple(tmp)
		_ = os.Remove(tmp)
		if rerr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": rerr.Error()})
		}
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unsupported file type (csv,xlsx)"})
	}

	if len(rows) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is empty"})
	}

	// Header mapping
	header := rows[0]
	col := mapHeaderIndexes(header)
	// minimally require Transaction ID and Transaction Date and Amount (any of the 3 amount columns)
	// Transaction ID from Wave is optional for our grouping logic now; we generate our own per invoice
	if _, ok := col["Transaction Date"]; !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing column: Transaction Date"})
	}

	inserted := 0
	skipped := 0
	duplicates := 0
	errorsList := []string{}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		for i := 1; i < len(rows); i++ {
			r := rows[i]
			get := func(key string) string {
				if idx, ok := col[key]; ok && idx < len(r) {
					return strings.TrimSpace(r[idx])
				}
				return ""
			}

			// Core identifiers
			waveTxID := normalizeSci(get("Transaction ID"))
			invoiceNo := get("Invoice Number")
			tDateStr := get("Transaction Date")
			// Generate deterministic TransactionID that is the same for all lines of the same bill
			// Prefer invoice number + year-month to reduce collisions across years
			txID := generateDeterministicTxnID(invoiceNo, tDateStr)

			// Build deterministic RowUID from key columns to avoid duplicates across multiple imports
			rowUID := strings.Join([]string{
				txID,
				invoiceNo,
				tDateStr,
				numberish(get("Amount (One column)")),
				numberish(get("Debit Amount (Two Column Approach)")),
				numberish(get("Credit Amount (Two Column Approach)")),
				get("Account Name"),
			}, "|")

			// Dedup check by RowUID
			var existing models.Bill
			if err := tx.Where("row_uid = ?", rowUID).First(&existing).Error; err == nil {
				duplicates++
				skipped++
				continue
			}

			// Parse dates
			tDate := parseDate(tDateStr)
			tAdded := parseDate(get("Transaction Date Added"))
			tMod := parseDate(get("Transaction Date Last Modified"))

			// Parse amounts (prefer explicit debit/credit; keep sign as-is for one-column)
			amountPtr := parseFloatPtr(numberish(get("Amount (One column)")))
			debitPtr := parseFloatPtr(numberish(get("Debit Amount (Two Column Approach)")))
			creditPtr := parseFloatPtr(numberish(get("Credit Amount (Two Column Approach)")))
			beforeTax := parseFloatPtr(numberish(get("Amount Before Sales Tax")))
			taxAmt := parseFloatPtr(numberish(get("Sales Tax Amount")))

			// Payment method heuristic from Account Name or Other Accounts
			pm := detectPaymentMethod(get("Account Name"), get("Other Accounts for this Transaction"), get("Notes / Memo"))

			// Build raw JSON of row for traceability
			rawMap := map[string]string{}
			for i2, h := range header {
				if i2 < len(r) {
					rawMap[h] = r[i2]
				}
			}
			rawBytes, _ := json.Marshal(rawMap)

			bill := models.Bill{
				Source:                      "wave",
				TransactionID:               txID,
				SourceTransactionID:         waveTxID,
				RowUID:                      rowUID,
				TransactionDate:             tDate,
				AccountName:                 get("Account Name"),
				TransactionDescription:      get("Transaction Description"),
				TransactionLineDescription:  get("Transaction Line Description"),
				Amount:                      amountPtr,
				DebitAmount:                 debitPtr,
				CreditAmount:                creditPtr,
				OtherAccount:                get("Other Accounts for this Transaction"),
				Customer:                    get("Customer"),
				InvoiceNumber:               get("Invoice Number"),
				NotesMemo:                   get("Notes / Memo"),
				AmountBeforeSalesTax:        beforeTax,
				SalesTaxAmount:              taxAmt,
				SalesTaxName:                get("Sales Tax Name"),
				TransactionDateAdded:        tAdded,
				TransactionDateLastModified: tMod,
				AccountGroup:                get("Account Group"),
				AccountType:                 get("Account Type"),
				AccountID:                   get("Account ID"),
				PaymentMethod:               pm,
				Raw:                         models.JSON(rawBytes),
			}

			if err := tx.Create(&bill).Error; err != nil {
				errorsList = append(errorsList, fmt.Sprintf("row %d: %v", i+1, err))
				continue
			}
			inserted++
		}
		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"file_name":    fh.Filename,
		"data_rows":    len(rows) - 1,
		"inserted":     inserted,
		"skipped":      skipped,
		"duplicates":   duplicates,
		"errors_count": len(errorsList),
		"errors":       errorsList,
	})
}

// Helpers (localized to this controller to avoid cross-file imports)
// generateDeterministicTxnID creates an application-level transaction ID used to group multiple lines of the same bill.
// Format: INV-{invoiceNo}-{YYYYMM}. If invoiceNo is empty, fallback to HASH-{YYYYMM}-{hash of description/date/amount}.
func generateDeterministicTxnID(invoiceNo, tDateStr string) string {
	inv := strings.TrimSpace(invoiceNo)
	ym := ""
	if t := parseDate(tDateStr); t != nil {
		ym = t.Format("200601")
	}
	if inv != "" {
		if ym != "" {
			return "INV-" + inv + "-" + ym
		}
		return "INV-" + inv
	}
	// Fallback: stable hash from available parts to keep grouping consistent if invoice missing
	key := strings.Join([]string{tDateStr, inv}, "|")
	// simple 32-bit sum hash to keep deterministic but short
	var sum uint32
	for i := 0; i < len(key); i++ {
		sum = sum*16777619 ^ uint32(key[i])
	}
	if ym != "" {
		return fmt.Sprintf("HASH-%s-%08x", ym, sum)
	}
	return fmt.Sprintf("HASH-%08x", sum)
}
func readCSVSimple(r io.Reader) ([][]string, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	var rows [][]string
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

func readXLSXSimple(path string) ([][]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sht := f.GetSheetName(0)
	if sht == "" {
		sht = "Sheet1"
	}
	data, err := f.GetRows(sht)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func mapHeaderIndexes(header []string) map[string]int {
	m := map[string]int{}
	for i, h := range header {
		key := strings.TrimSpace(h)
		m[key] = i
	}
	return m
}

func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{"1/2/2006", "01/02/2006", "02/01/2006", "2006-01-02", time.RFC3339}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	// Try 1/2/06
	if t, err := time.Parse("1/2/06", s); err == nil {
		return &t
	}
	return nil
}

func parseFloatPtr(s string) *float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return &v
	}
	return nil
}

func numberish(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// If value like 2.17E+18, try to reconstruct plain integer string
	if strings.ContainsAny(s, "Ee") && strings.Contains(s, "+") {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			// format without exponent
			return fmt.Sprintf("%.0f", f)
		}
	}
	return s
}

func normalizeSci(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// remove commas and quotes
	s = strings.Trim(s, "\"")
	sNoComma := strings.ReplaceAll(s, ",", "")
	if strings.ContainsAny(sNoComma, "Ee") {
		// Use big.Float to avoid precision loss for 64-bit floats
		var bf big.Float
		if _, ok := bf.SetString(sNoComma); ok {
			bi, _ := bf.Int(nil)
			return bi.String()
		}
	}
	return sNoComma
}

func detectPaymentMethod(accountName, otherAccount, notes string) string {
	chk := strings.ToLower(accountName + " " + otherAccount + " " + notes)
	// Thai keywords and English
	if strings.Contains(chk, "เงินสด") || strings.Contains(chk, "cash") {
		return "cash"
	}
	if strings.Contains(chk, "เดบิต") || strings.Contains(chk, "debit") {
		return "debit_card"
	}
	if strings.Contains(chk, "เครดิต") || strings.Contains(chk, "credit") {
		return "credit_card"
	}
	if strings.Contains(chk, "โอน") || strings.Contains(chk, "transfer") || strings.Contains(chk, "scb") || strings.Contains(chk, "kbank") {
		return "transfer"
	}
	return "unknown"
}
