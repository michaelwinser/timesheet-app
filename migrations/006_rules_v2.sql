-- Migration 006: Rules System v2
--
-- Changes:
-- 1. Add query column to classification_rules for text-based query syntax
-- 2. Add is_generated flag to track auto-generated rules from project fingerprints
-- 3. Add display_order for drag-to-reorder
-- 4. Add fingerprint columns to projects table (domains, emails, keywords)
-- 5. Drop rule_conditions table (conditions now stored in query string)

-- =============================================================================
-- PROJECTS: Add fingerprint columns for auto-rule generation
-- =============================================================================

-- Domains that match this project (JSON array of strings)
ALTER TABLE projects ADD COLUMN IF NOT EXISTS fingerprint_domains JSONB DEFAULT '[]';

-- Email addresses that match this project (JSON array of strings)
ALTER TABLE projects ADD COLUMN IF NOT EXISTS fingerprint_emails JSONB DEFAULT '[]';

-- Title keywords that match this project (JSON array of strings)
ALTER TABLE projects ADD COLUMN IF NOT EXISTS fingerprint_keywords JSONB DEFAULT '[]';

COMMENT ON COLUMN projects.fingerprint_domains IS 'Domain patterns that auto-classify to this project';
COMMENT ON COLUMN projects.fingerprint_emails IS 'Email addresses that auto-classify to this project';
COMMENT ON COLUMN projects.fingerprint_keywords IS 'Title keywords that auto-classify to this project';

-- =============================================================================
-- CLASSIFICATION_RULES: Add query-based matching
-- =============================================================================

-- Add query column for text-based query syntax
ALTER TABLE classification_rules ADD COLUMN IF NOT EXISTS query TEXT;

-- Add flag to track auto-generated rules from project fingerprints
ALTER TABLE classification_rules ADD COLUMN IF NOT EXISTS is_generated BOOLEAN DEFAULT false;

-- Add display_order for drag-to-reorder (separate from priority for evaluation)
ALTER TABLE classification_rules ADD COLUMN IF NOT EXISTS display_order INTEGER DEFAULT 0;

COMMENT ON COLUMN classification_rules.query IS 'Text-based query syntax: domain:foo.com title:"meeting"';
COMMENT ON COLUMN classification_rules.is_generated IS 'True if auto-generated from project fingerprint';
COMMENT ON COLUMN classification_rules.display_order IS 'Order within project group for UI display';

-- =============================================================================
-- Drop existing rules and conditions (clean break per PRD)
-- =============================================================================

-- Delete all existing rule conditions
DELETE FROM rule_conditions;

-- Delete all existing rules
DELETE FROM classification_rules;

-- Now we can drop the rule_conditions table
DROP TABLE IF EXISTS rule_conditions;

-- =============================================================================
-- Update indexes
-- =============================================================================

-- Index for efficient rule lookup by target_type (DNA rules evaluated separately)
CREATE INDEX IF NOT EXISTS idx_classification_rules_target_type
    ON classification_rules(user_id, target_type, priority DESC);

-- Index for generated rules lookup
CREATE INDEX IF NOT EXISTS idx_classification_rules_generated
    ON classification_rules(user_id, is_generated);
