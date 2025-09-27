# Bills System Guide

This guide explains the bills import pipeline (from Wave), the native Bills API, the data model, and how to manage deposits and installments.

## Overview
- Import transactions exported from Wave (CSV/XLSX). The app normalizes and deduplicates rows.
- The app generates its own `transaction_id` per invoice and month to group multiple lines of the same bill.
- The original Wave Transaction ID is stored in `source_transaction_id` for audit/provenance.
- We support semantic bill types like `deposit` and `installment` to make it easy to understand billing flows.

## Data Model

`models.Bill` main fields:
- `source` (string): `"wave"` for imports, `"manual"` for user-created bills.
- `transaction_id` (string, index): App-generated deterministic ID used to group multiple lines of a bill.
  - Format: `INV-<invoiceNumber>-<YYYYMM>` if `invoice_number` is present.
  - Fallback: `HASH-<YYYYMM>-<hash>` if invoice is missing.
- `source_transaction_id` (string, index): Original Wave Transaction ID, normalized from scientific notation (if any).
- `row_uid` (string, unique): Deterministic unique identifier per line to deduplicate imports and repeated submissions.
- `transaction_date` (datetime): Date of the transaction.
- `bill_type` (enum): `normal` (default), `deposit`, `installment`, `payment`, `adjustment`.
- `installment_no` (*int): Current installment number.
- `total_installments` (*int): Total number of installments for the plan.
- Wave columns: `account_name`, `transaction_description`, `transaction_line_description`, `amount`, `debit_amount`, `credit_amount`, `other_account`, `customer`, `invoice_number`, `notes_memo`, `amount_before_sales_tax`, `sales_tax_amount`, `sales_tax_name`, `transaction_date_added`, `transaction_date_last_modified`, `account_group`, `account_type`, `account_id`.
- Derived: `payment_method` (enum: `cash`, `debit_card`, `credit_card`, `transfer`, `other`, `unknown`), `currency`, `status`, `due_date`, `paid_date`.
- `raw` (json): Original row capture for traceability.

Bill status values:
- `Paid` - the bill has been fully paid and reconciled.
- `Unpaid` - the bill is recorded but not yet paid. (default for manual bills)
- `Overdue` - the bill has passed its due date and remains unpaid.
- `Partially Paid` - some payment(s) have been made but the bill is not fully settled.

This allows re-importing the same Wave file safely and idempotent manual submissions.

### Manual Create Example (Deposit)
```
POST /api/bills
{
  "invoice_number": "INV-2025-009",
  "transaction_date": "2025-09-21",
  "bill_type": "deposit",
  "customer": "ACME Co.",
  "currency": "THB",
  "lines": [
    {
      "account_name": "Cash",
      "description": "Deposit for course ENG-101",
      "amount": 5000.00,
      "notes": "รับมัดจำ"
    }
  ]
}
```
Response:
```
{ "success": true, "transaction_id": "INV-INV-2025-009-202509" }
```

### Manual Create Example (Installment)
```
POST /api/bills
{
  "invoice_number": "INV-2025-010",
  "transaction_date": "2025-09-21",
  "bill_type": "installment",
  "installment_no": 1,
  "total_installments": 3,
  "customer": "ACME Co.",
  "currency": "THB",
  "lines": [
    { "account_name": "Bank - SCB", "description": "1/3 Payment", "amount": 6000.00 }
  ]
}
```
# Bills System Guide

This guide explains the bills import pipeline (from Wave), the native Bills API, the data model, and how to manage deposits and installments.

## Overview
- Import transactions exported from Wave (CSV/XLSX). The app normalizes and deduplicates rows.
- The app generates its own `transaction_id` per invoice and month to group multiple lines of the same bill.
- The original Wave Transaction ID is stored in `source_transaction_id` for audit/provenance.
- We support semantic bill types like `deposit` and `installment` to make it easy to understand billing flows.

## Table of contents
- Data model
- Deterministic Transaction ID
- Deduplication strategy
- Importer behavior (Wave)
- Native Bills API
- Bill statuses and lifecycle
- Examples
- Tips and conventions
- Troubleshooting & migrations

## Data Model

`models.Bill` main fields:
- `source` (string): `"wave"` for imports, `"manual"` for user-created bills.
- `transaction_id` (string, index): App-generated deterministic ID used to group multiple lines of a bill.
  - Format: `INV-<invoiceNumber>-<YYYYMM>` if `invoice_number` is present.
  - Fallback: `HASH-<YYYYMM>-<hash>` if invoice is missing.
- `source_transaction_id` (string, index): Original Wave Transaction ID, normalized from scientific notation (if any).
- `row_uid` (string, unique): Deterministic unique identifier per row to deduplicate imports and repeated submissions.
- `transaction_date` (datetime): Date of the transaction.
- `bill_type` (enum): `normal` (default), `deposit`, `installment`, `payment`, `adjustment`.
- `installment_no` (*int): Current installment number.
- `total_installments` (*int): Total number of installments for the plan.
- Wave columns: `account_name`, `transaction_description`, `transaction_line_description`, `amount`, `debit_amount`, `credit_amount`, `other_account`, `customer`, `invoice_number`, `notes_memo`, `amount_before_sales_tax`, `sales_tax_amount`, `sales_tax_name`, `transaction_date_added`, `transaction_date_last_modified`, `account_group`, `account_type`, `account_id`.
- Derived: `payment_method` (enum: `cash`, `debit_card`, `credit_card`, `transfer`, `other`, `unknown`), `currency`, `status`, `due_date`, `paid_date`.
- `raw` (json): Original row capture for traceability.

### Bill statuses
The application tracks a bill `status` that indicates its payment/reconciliation state. Allowed values:
- `Paid` - the bill has been fully paid and reconciled.
- `Unpaid` - the bill is recorded but not yet paid. (default for manual bills)
- `Overdue` - the bill has passed its due date and remains unpaid.
- `Partially Paid` - some payment(s) have been made but the bill is not fully settled.

These statuses are enforced at the application level and validated on PATCH to avoid accidental free-form values.

## Deterministic Transaction ID

The internal `transaction_id` is generated to group lines from the same bill. This is used for reporting and retrieving all lines related to a single invoice for a given month.

- If `invoice_number` is provided: `INV-<invoiceNumber>-<YYYYMM>` where `YYYYMM` is derived from the transaction date.
- If no invoice number: compute a stable hash of date and invoice parts and produce `HASH-<YYYYMM>-<hash>`.

This ensures all rows for the same invoice and month share a single grouping ID.

## Deduplication strategy

To avoid duplicate inserts:
- We compute `row_uid` by concatenating stable fields such as `transaction_id`, `invoice_number`, `transaction_date`, amounts, `account_name`, and descriptions for manual rows.
- Before insert, we check for an existing row with the same `row_uid`. If found, the new row is skipped.

This makes re-importing the same Wave file idempotent and prevents duplicate manual entries.

## Importer behavior (Wave)

Endpoint: `POST /api/import/bills` with `multipart/form-data` containing `file` (CSV or XLSX).

- Header mapping: The importer maps typical Wave export columns. Unknown columns are preserved in `raw` JSON.
- `source_transaction_id`: Normalized value of Wave `Transaction ID`. Scientific notation (e.g., `2.17E+18`) is converted into a plain integer string using high-precision parsing.
- Amount parsing: Supports single-column (`amount`) or two-column (`debit_amount`/`credit_amount`) formats. Commas are ignored. Negative values are supported.
- Payment method detection: Heuristics based on `account_name`, `other_account`, and `notes_memo` (Thai/English keywords) to map to `cash`, `debit_card`, `credit_card`, `transfer`, `other`, or `unknown`.
- Date parsing: Supports common formats: `1/2/2006`, `01/02/2006`, `02/01/2006`, `2006-01-02`, and RFC3339.
- Deduplication: Uses `row_uid`. Duplicate rows are counted and skipped; the importer returns stats on inserted/skipped/duplicates.

Example response from importer:

```
{
  "success": true,
  "file_name": "wave-export-2025-09.xlsx",
  "data_rows": 123,
  "inserted": 119,
  "skipped": 4,
  "duplicates": 4,
  "errors_count": 0,
  "errors": []
}
```

## Native Bills API

All endpoints require Owner/Admin role and live under `/api/bills`.

- List bills: `GET /api/bills?page=1&page_size=20&invoice=...&transaction_id=...&bill_type=...&customer=...&account=...&date_from=YYYY-MM-DD&date_to=YYYY-MM-DD`
- Get by ID: `GET /api/bills/:id`
- Get by transaction: `GET /api/bills/by-transaction/:transactionId`
- Get by invoice: `GET /api/bills/by-invoice/:invoice`
- Create (manual): `POST /api/bills` with body `{ invoice_number, transaction_date, bill_type?, installment_no?, total_installments?, transaction_id?, customer?, currency?, lines: [ ... ] }`
- Patch: `PATCH /api/bills/:id` to update `status`, `due_date`, `paid_date`, `notes_memo`, `bill_type`, `installment_no`, `total_installments`.
- Delete: `DELETE /api/bills/:id` (soft delete).

Notes on List filters:
- `invoice` filters by `invoice_number` exact match.
- `transaction_id` filters by the app-generated grouping id.
- `bill_type`, `customer`, `account` behave as expected (partial matches for strings).
- `date_from`/`date_to` filter on the `transaction_date` (inclusive on the range).

### Create (manual)

- `transaction_date` is required for manual creation (format: YYYY-MM-DD). If you omit `transaction_id`, it will be generated from `invoice_number` + YYYYMM using the same logic as the importer.
- `rows` for a transaction are created as separate `models.Bill` lines (one-per-line). `row_uid` is generated for deduplication.
- Manual-created bills default to `status = "Unpaid"` unless you immediately PATCH them.

### Patch (update)

Allowed updates: `status`, `due_date`, `paid_date`, `notes_memo`, `bill_type`, `installment_no`, `total_installments`.
- `status` accepts only: `Paid`, `Unpaid`, `Overdue`, `Partially Paid`. Invalid values return HTTP 400.
- `due_date`/`paid_date` should be provided as YYYY-MM-DD (or RFC3339). The server parses these and stores as timestamps.

### Delete
- Soft-delete (standard GORM Delete) — lines can be filtered out by default queries if your frontend respects soft-deletes.

## Examples

### Manual Create Example (Deposit)

```
POST /api/bills
{
  "invoice_number": "INV-2025-009",
  "transaction_date": "2025-09-21",
  "bill_type": "deposit",
  "customer": "ACME Co.",
  "currency": "THB",
  "lines": [
    {
      "account_name": "Cash",
      "description": "Deposit for course ENG-101",
      "amount": 5000.00,
      "notes": "รับมัดจำ"
    }
  ]
}
```
Response:
```
{ "success": true, "transaction_id": "INV-INV-2025-009-202509" }
```

### Manual Create Example (Installment)

```
POST /api/bills
{
  "invoice_number": "INV-2025-010",
  "transaction_date": "2025-09-21",
  "bill_type": "installment",
  "installment_no": 1,
  "total_installments": 3,
  "customer": "ACME Co.",
  "currency": "THB",
  "lines": [
    { "account_name": "Bank - SCB", "description": "1/3 Payment", "amount": 6000.00 }
  ]
}
```

### Update Bill (Mark Paid)

```
PATCH /api/bills/:id
Content-Type: application/json

{
  "status": "Paid",
  "paid_date": "2025-09-25",
  "notes_memo": "paid via transfer"
}
```

## Tips and conventions
- Keep `invoice_number` consistent across installments for the same plan to keep grouping stable.
- Use `Get by transaction` for all lines of the same grouped bill; use `Get by invoice` when analyzing customer invoices.
- Prefer `debit_amount`/`credit_amount` for two-column imports; `amount` is supported as a generic value as well.
- Use the `status` + `due_date` fields to implement simple aging/AR views.
- Prefer patching `paid_date` when a bill is settled so downstream reports can rely on that timestamp.

## Troubleshooting & migrations
- Migration errors on JSON fields: This project sets JSON defaults at the application level; existing invalid values are sanitized during migration.
- Import shows scientific notation IDs: The importer normalizes to plain integers into `source_transaction_id`.
- Duplicate rows on re-import: Confirm critical columns match exactly; minor text differences will produce a new `row_uid`.

### Database enum change notes
If your database enforces ENUM types at the SQL level (common in MySQL/Postgres setups), altering the `status` enum in the code may require a migration step:
- Map existing values to the new set (e.g., any legacy `record` -> `Unpaid`) before you change the column type to the new enum.
- Example (Postgres):
  - ALTER TABLE bills ALTER COLUMN status TYPE text;
  - UPDATE bills SET status = 'Unpaid' WHERE status = 'record';
  - Then apply a CHECK constraint or recreate an enum type and alter the column back to the enum.

If you want, I can prepare a small migration file for your DB engine (Postgres/MySQL/MariaDB). Tell me which DB you use and I will generate the SQL migration.

---

If you'd like adjustments (example payloads, different default mapping, or case-insensitive status handling), tell me which you prefer and I will update the code and docs accordingly.
