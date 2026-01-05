-- API Keys for programmatic access (MCP, CLI, integrations)
-- Keys are tied to users and have the same permissions as the user

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    -- Store SHA-256 hash of the key, not the key itself
    key_hash VARCHAR(64) NOT NULL,
    -- First 8 chars of key for display (e.g., "ts_abc123...")
    key_prefix VARCHAR(12) NOT NULL,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Each user can have multiple keys, but names should be unique per user
    UNIQUE(user_id, name)
);

-- Index for key lookup (used on every authenticated request)
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);

-- Index for listing user's keys
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
