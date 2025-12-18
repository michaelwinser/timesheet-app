-- Migration: Add short_code to projects for invoice prefixes
-- This short code (2-3 letters) is used as the invoice number prefix

ALTER TABLE projects ADD COLUMN IF NOT EXISTS short_code VARCHAR(10);

COMMENT ON COLUMN projects.short_code IS 'Short code (2-3 letters) used as invoice number prefix';
