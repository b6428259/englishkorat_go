# Bills Import (Wave)

This endpoint imports accounting transactions exported from Wave (CSV/XLSX) and stores them as `bills` for reporting and reconciliation.

- Endpoint: POST /api/import/bills
- Auth: owner/admin
- Body: multipart/form-data with field `file`
- Format: Wave export columns. At minimum requires `Transaction Date`. Amount columns are optional but recommended.

Deduplication
- We compute a deterministic RowUID from a generated Transaction ID (see below), Invoice Number, Transaction Date, Amount columns, and Account Name.
- If a row with the same RowUID already exists, it will be skipped. This allows multiple imports over time without duplicates.z 

Payment methods supported
- cash (เงินสด)
- debit_card (เดบิต)
- credit_card (เครดิต)
- transfer (โอน, bank transfers; heuristic detects SCB/KBANK mentions)
- unknown (fallback)

Response sample
{
  "success": true,
  "file_name": "wave-export-2025-01.csv",
  "data_rows": 120,
  "inserted": 118,
  "skipped": 2,
  "duplicates": 2,
  "errors_count": 0
}

Notes
- We keep the entire original row as JSON in `bills.raw` for traceability.
- The model is auto-migrated; no manual SQL migration needed.
- Transaction ID policy: We generate a new application-level Transaction ID that is shared by all lines of the same invoice.
  - Format: `INV-<InvoiceNumber>-<YYYYMM>` (based on the row's Transaction Date)
  - If `Invoice Number` is blank, we fallback to a stable hash ID using date to keep grouping deterministic.
- We also store the original Wave `Transaction ID` (normalized from scientific notation) in `source_transaction_id` for reference.
