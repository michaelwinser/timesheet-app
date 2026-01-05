-- MCP OAuth sessions for authorization code flow with PKCE
CREATE TABLE mcp_oauth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- State parameter for CSRF protection
    state VARCHAR(128) NOT NULL UNIQUE,
    -- PKCE code challenge
    code_challenge VARCHAR(128) NOT NULL,
    code_challenge_method VARCHAR(10) NOT NULL DEFAULT 'S256',
    -- Redirect URI for the MCP client
    redirect_uri TEXT NOT NULL,
    -- Authorization code (set after user authenticates)
    auth_code VARCHAR(64) UNIQUE,
    auth_code_expires_at TIMESTAMPTZ,
    -- User who authorized (set after authentication)
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Sessions expire after 10 minutes if not completed
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '10 minutes'
);

-- Index for looking up by state during OAuth callback
CREATE INDEX idx_mcp_oauth_sessions_state ON mcp_oauth_sessions(state);

-- Index for looking up by auth_code during token exchange
CREATE INDEX idx_mcp_oauth_sessions_auth_code ON mcp_oauth_sessions(auth_code);

-- MCP access tokens (issued after successful OAuth)
CREATE TABLE mcp_access_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Token is stored as SHA-256 hash (like API keys)
    token_hash VARCHAR(64) NOT NULL,
    token_prefix VARCHAR(12) NOT NULL,
    -- Tokens expire after 24 hours by default
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_mcp_access_tokens_hash ON mcp_access_tokens(token_hash);
CREATE INDEX idx_mcp_access_tokens_user ON mcp_access_tokens(user_id);

-- Clean up expired sessions and tokens periodically
-- (Can be done via cron or app-level cleanup)
