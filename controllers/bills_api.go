package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/models"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// BillsController provides RESTful endpoints for managing bills
type BillsController struct{}

// ListBills GET /api/bills
// Query params: page, page_size, invoice, transaction_id, bill_type, date_from, date_to, customer, account
func (bc *BillsController) ListBills(c *fiber.Ctx) error {
	db := database.DB

	// Pagination defaults
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	q := db.Model(&models.Bill{})

	if v := strings.TrimSpace(c.Query("invoice")); v != "" {
		q = q.Where("invoice_number = ?", v)
	}
	if v := strings.TrimSpace(c.Query("transaction_id")); v != "" {
		q = q.Where("transaction_id = ?", v)
	}
	if v := strings.TrimSpace(c.Query("bill_type")); v != "" {
		q = q.Where("bill_type = ?", v)
	}
	if v := strings.TrimSpace(c.Query("customer")); v != "" {
		q = q.Where("customer LIKE ?", "%"+v+"%")
	}
	if v := strings.TrimSpace(c.Query("account")); v != "" {
		q = q.Where("account_name LIKE ?", "%"+v+"%")
	}

	// Date range filter (TransactionDate)
	if v := strings.TrimSpace(c.Query("date_from")); v != "" {
		if t := parseAPIDate(v); t != nil {
			q = q.Where("transaction_date >= ?", t)
		}
	}
	if v := strings.TrimSpace(c.Query("date_to")); v != "" {
		if t := parseAPIDate(v); t != nil {
			// include entire day
			tend := t.Add(24*time.Hour - time.Nanosecond)
			q = q.Where("transaction_date <= ?", tend)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	var items []models.Bill
	if err := q.Order("transaction_date DESC, id DESC").Limit(pageSize).Offset(offset).Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     items,
	})
}

// GetBill GET /api/bills/:id
func (bc *BillsController) GetBill(c *fiber.Ctx) error {
	id := c.Params("id")
	var b models.Bill
	if err := database.DB.First(&b, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "bill not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(b)
}

// GetByTransaction GET /api/bills/by-transaction/:transactionId
func (bc *BillsController) GetByTransaction(c *fiber.Ctx) error {
	txid := c.Params("transactionId")
	var items []models.Bill
	if err := database.DB.Where("transaction_id = ?", txid).Order("id").Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"transaction_id": txid, "items": items})
}

// GetByInvoice GET /api/bills/by-invoice/:invoice
func (bc *BillsController) GetByInvoice(c *fiber.Ctx) error {
	inv := c.Params("invoice")
	var items []models.Bill
	if err := database.DB.Where("invoice_number = ?", inv).Order("transaction_date, id").Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"invoice_number": inv, "items": items})
}

// CreateBill POST /api/bills
// Accepts a manual bill payload for creating your own bills (not from import)
// This supports deposits and installments. The payload can be a single line or multiple lines.
// If transaction_id is empty, we will generate it based on invoice_number + YYYYMM (same logic as import)
func (bc *BillsController) CreateBill(c *fiber.Ctx) error {
	var req struct {
		InvoiceNumber     string `json:"invoice_number"`
		TransactionDate   string `json:"transaction_date"` // YYYY-MM-DD
		BillType          string `json:"bill_type"`
		InstallmentNo     *int   `json:"installment_no"`
		TotalInstallments *int   `json:"total_installments"`
		TransactionID     string `json:"transaction_id"`
		Customer          string `json:"customer"`
		Currency          string `json:"currency"`
		Lines             []struct {
			AccountName     string   `json:"account_name"`
			Description     string   `json:"description"`
			LineDescription string   `json:"line_description"`
			Amount          *float64 `json:"amount"`
			DebitAmount     *float64 `json:"debit_amount"`
			CreditAmount    *float64 `json:"credit_amount"`
			Notes           string   `json:"notes"`
		} `json:"lines"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}

	if strings.TrimSpace(req.TransactionDate) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "transaction_date is required"})
	}
	tdate := parseAPIDate(req.TransactionDate)
	if tdate == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid transaction_date format (use YYYY-MM-DD)"})
	}

	txID := strings.TrimSpace(req.TransactionID)
	if txID == "" {
		txID = generateDeterministicTxnID(req.InvoiceNumber, tdate.Format("2006-01-02"))
	}

	bt := strings.TrimSpace(req.BillType)
	if bt == "" {
		bt = "normal"
	}

	// Build rows
	if len(req.Lines) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "lines are required"})
	}

	// Insert within a transaction
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		for _, ln := range req.Lines {
			rowUID := strings.Join([]string{
				txID,
				req.InvoiceNumber,
				tdate.Format("2006-01-02"),
				fstr(ln.Amount),
				fstr(ln.DebitAmount),
				fstr(ln.CreditAmount),
				ln.AccountName,
				ln.Description,
			}, "|")

			// Dedup by row_uid
			var ex models.Bill
			if err := tx.Where("row_uid = ?", rowUID).First(&ex).Error; err == nil {
				continue // skip exact duplicates
			}

			bill := models.Bill{
				Source:                     "manual",
				TransactionID:              txID,
				RowUID:                     rowUID,
				TransactionDate:            tdate,
				BillType:                   bt,
				InstallmentNo:              req.InstallmentNo,
				TotalInstallments:          req.TotalInstallments,
				AccountName:                ln.AccountName,
				TransactionDescription:     ln.Description,
				TransactionLineDescription: ln.LineDescription,
				Amount:                     ln.Amount,
				DebitAmount:                ln.DebitAmount,
				CreditAmount:               ln.CreditAmount,
				NotesMemo:                  ln.Notes,
				Customer:                   req.Customer,
				Currency:                   req.Currency,
				InvoiceNumber:              req.InvoiceNumber,
				Status:                     "Unpaid",
			}
			if err := tx.Create(&bill).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "transaction_id": txID})
}

// PatchBill PATCH /api/bills/:id
// Allows updating status, due_date, paid_date, notes_memo, bill_type, installment fields
func (bc *BillsController) PatchBill(c *fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		Status            *string `json:"status"`
		DueDate           *string `json:"due_date"`  // YYYY-MM-DD
		PaidDate          *string `json:"paid_date"` // YYYY-MM-DD
		NotesMemo         *string `json:"notes_memo"`
		BillType          *string `json:"bill_type"`
		InstallmentNo     *int    `json:"installment_no"`
		TotalInstallments *int    `json:"total_installments"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}

	var b models.Bill
	if err := database.DB.First(&b, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "bill not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	updates := map[string]interface{}{}
	if req.Status != nil {
		// validate status against allowed values
		s := strings.TrimSpace(*req.Status)
		allowed := map[string]bool{"Paid": true, "Unpaid": true, "Overdue": true, "Partially Paid": true}
		if !allowed[s] {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid status value"})
		}
		updates["status"] = s
	}
	if req.NotesMemo != nil {
		updates["notes_memo"] = *req.NotesMemo
	}
	if req.BillType != nil {
		updates["bill_type"] = *req.BillType
	}
	if req.InstallmentNo != nil {
		updates["installment_no"] = *req.InstallmentNo
	}
	if req.TotalInstallments != nil {
		updates["total_installments"] = *req.TotalInstallments
	}
	if req.DueDate != nil {
		if t := parseAPIDate(*req.DueDate); t != nil {
			updates["due_date"] = t
		}
	}
	if req.PaidDate != nil {
		if t := parseAPIDate(*req.PaidDate); t != nil {
			updates["paid_date"] = t
		}
	}

	if err := database.DB.Model(&b).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

// DeleteBill DELETE /api/bills/:id (soft delete)
func (bc *BillsController) DeleteBill(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := database.DB.Delete(&models.Bill{}, id).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

// parseAPIDate parses YYYY-MM-DD (and a few common variants)
func parseAPIDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{"2006-01-02", time.RFC3339, "02/01/2006", "01/02/2006"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}

func fstr(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *p)
}
