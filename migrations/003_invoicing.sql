-- Migration: Invoicing feature
--
-- Adds:
-- 1. Invoices table for invoice headers
-- 2. Invoice line items table for individual entries
-- 3. Invoice reference on time_entries
-- 4. Google Sheets spreadsheet link on projects

-- =============================================================================
-- PROJECTS - Add Google Sheets spreadsheet link
-- =============================================================================
ALTER TABLE projects ADD COLUMN IF NOT EXISTS sheets_spreadsheet_id VARCHAR(255);
ALTER TABLE projects ADD COLUMN IF NOT EXISTS sheets_spreadsheet_url TEXT;

COMMENT ON COLUMN projects.sheets_spreadsheet_id IS 'Google Sheets ID for invoice exports';
COMMENT ON COLUMN projects.sheets_spreadsheet_url IS 'URL to the Google Sheets document';

-- =============================================================================
-- INVOICES - Invoice header records
-- =============================================================================
CREATE TABLE IF NOT EXISTS invoices (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Invoice identification
    invoice_number VARCHAR(50) NOT NULL,

    -- Date range covered
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,

    -- Invoice metadata
    invoice_date DATE NOT NULL DEFAULT CURRENT_DATE,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',  -- draft, finalized, paid

    -- Calculated totals (denormalized for quick access)
    total_hours REAL NOT NULL DEFAULT 0,
    total_amount DECIMAL(10, 2) NOT NULL DEFAULT 0,

    -- Google Sheets export tracking
    sheets_spreadsheet_id VARCHAR(255),  -- May differ from project if created before project had one
    sheets_worksheet_id INTEGER,          -- Worksheet (sheet) ID within spreadsheet
    last_exported_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Invoice numbers unique per user
    UNIQUE(user_id, invoice_number)
);

CREATE INDEX IF NOT EXISTS idx_invoices_user ON invoices(user_id);
CREATE INDEX IF NOT EXISTS idx_invoices_project ON invoices(user_id, project_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(user_id, status);
CREATE INDEX IF NOT EXISTS idx_invoices_date ON invoices(user_id, invoice_date DESC);

COMMENT ON TABLE invoices IS 'Invoice headers with totals and export tracking';
COMMENT ON COLUMN invoices.invoice_number IS 'Unique invoice number per user (format: PROJECT-YEAR-SEQ)';
COMMENT ON COLUMN invoices.status IS 'Invoice status: draft, finalized, or paid';
COMMENT ON COLUMN invoices.sheets_worksheet_id IS 'Google Sheets worksheet (tab) ID for this invoice';

-- =============================================================================
-- INVOICE_LINE_ITEMS - Individual entries on an invoice
-- =============================================================================
CREATE TABLE IF NOT EXISTS invoice_line_items (
    id SERIAL PRIMARY KEY,
    invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    time_entry_id INTEGER REFERENCES time_entries(id) ON DELETE SET NULL,

    -- Snapshot of time entry data at invoice time
    entry_date DATE NOT NULL,
    description TEXT,
    hours REAL NOT NULL,
    rate DECIMAL(10, 2) NOT NULL,  -- Bill rate at time of invoicing
    amount DECIMAL(10, 2) NOT NULL,  -- hours * rate

    -- Track if source entry was deleted
    is_orphaned BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_invoice_line_items_invoice ON invoice_line_items(invoice_id);
CREATE INDEX IF NOT EXISTS idx_invoice_line_items_entry ON invoice_line_items(time_entry_id);

COMMENT ON TABLE invoice_line_items IS 'Individual line items on an invoice, snapshot of time entry data';
COMMENT ON COLUMN invoice_line_items.rate IS 'Bill rate at time of invoice creation (snapshot)';
COMMENT ON COLUMN invoice_line_items.is_orphaned IS 'True if the source time entry was deleted after invoicing';

-- =============================================================================
-- TIME_ENTRIES - Add invoice reference
-- =============================================================================
ALTER TABLE time_entries ADD COLUMN IF NOT EXISTS invoice_id INTEGER REFERENCES invoices(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_time_entries_invoice ON time_entries(invoice_id);

COMMENT ON COLUMN time_entries.invoice_id IS 'Invoice this entry is included in (NULL = unbilled)';
