package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrOAuthSessionNotFound  = errors.New("OAuth session not found")
	ErrOAuthSessionExpired   = errors.New("OAuth session expired")
	ErrInvalidAuthCode       = errors.New("invalid authorization code")
	ErrAuthCodeExpired       = errors.New("authorization code expired")
	ErrCodeChallengeInvalid  = errors.New("code verifier does not match challenge")
	ErrMCPTokenNotFound      = errors.New("MCP access token not found")
	ErrMCPTokenExpired       = errors.New("MCP access token expired")
)

// MCPOAuthSession represents an in-progress OAuth authorization
type MCPOAuthSession struct {
	ID                  uuid.UUID
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	RedirectURI         string
	AuthCode            *string
	AuthCodeExpiresAt   *time.Time
	UserID              *uuid.UUID
	CreatedAt           time.Time
	ExpiresAt           time.Time
}

// MCPAccessToken represents an issued access token
type MCPAccessToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	TokenPrefix string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// MCPAccessTokenWithSecret includes the raw token (only at creation)
type MCPAccessTokenWithSecret struct {
	MCPAccessToken
	Token string
}

// MCPOAuthStore provides PostgreSQL-backed MCP OAuth storage
type MCPOAuthStore struct {
	pool *pgxpool.Pool
}

// NewMCPOAuthStore creates a new MCP OAuth store
func NewMCPOAuthStore(pool *pgxpool.Pool) *MCPOAuthStore {
	return &MCPOAuthStore{pool: pool}
}

// generateState creates a random state parameter
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateAuthCode creates a random authorization code
func generateAuthCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateMCPToken creates a new MCP access token
// Format: mcp_<32 random hex chars>
func generateMCPToken() (token string, prefix string, hash string, err error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", "", err
	}

	token = "mcp_" + hex.EncodeToString(randomBytes)
	prefix = token[:12] // "mcp_" + first 8 hex chars

	hashBytes := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(hashBytes[:])

	return token, prefix, hash, nil
}

// hashToken computes SHA-256 hash of a token
func hashToken(token string) string {
	hashBytes := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hashBytes[:])
}

// verifyCodeChallenge verifies PKCE code_verifier against code_challenge
func verifyCodeChallenge(verifier, challenge, method string) bool {
	if method != "S256" {
		return false
	}

	// S256: BASE64URL(SHA256(code_verifier)) == code_challenge
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == challenge
}

// CreateSession starts a new OAuth authorization session
func (s *MCPOAuthStore) CreateSession(ctx context.Context, codeChallenge, codeChallengeMethod, redirectURI string) (*MCPOAuthSession, error) {
	state, err := generateState()
	if err != nil {
		return nil, err
	}

	session := &MCPOAuthSession{
		ID:                  uuid.New(),
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		RedirectURI:         redirectURI,
		CreatedAt:           time.Now().UTC(),
		ExpiresAt:           time.Now().UTC().Add(10 * time.Minute),
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO mcp_oauth_sessions (id, state, code_challenge, code_challenge_method, redirect_uri, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, session.ID, session.State, session.CodeChallenge, session.CodeChallengeMethod,
	   session.RedirectURI, session.CreatedAt, session.ExpiresAt)

	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetSessionByState retrieves an OAuth session by state parameter
func (s *MCPOAuthStore) GetSessionByState(ctx context.Context, state string) (*MCPOAuthSession, error) {
	var session MCPOAuthSession
	err := s.pool.QueryRow(ctx, `
		SELECT id, state, code_challenge, code_challenge_method, redirect_uri,
		       auth_code, auth_code_expires_at, user_id, created_at, expires_at
		FROM mcp_oauth_sessions
		WHERE state = $1
	`, state).Scan(
		&session.ID, &session.State, &session.CodeChallenge, &session.CodeChallengeMethod,
		&session.RedirectURI, &session.AuthCode, &session.AuthCodeExpiresAt,
		&session.UserID, &session.CreatedAt, &session.ExpiresAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOAuthSessionNotFound
		}
		return nil, err
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, ErrOAuthSessionExpired
	}

	return &session, nil
}

// CompleteAuthorization sets the user and generates an auth code for the session
func (s *MCPOAuthStore) CompleteAuthorization(ctx context.Context, state string, userID uuid.UUID) (authCode string, redirectURI string, err error) {
	session, err := s.GetSessionByState(ctx, state)
	if err != nil {
		return "", "", err
	}

	authCode, err = generateAuthCode()
	if err != nil {
		return "", "", err
	}

	authCodeExpires := time.Now().UTC().Add(5 * time.Minute)

	_, err = s.pool.Exec(ctx, `
		UPDATE mcp_oauth_sessions
		SET auth_code = $1, auth_code_expires_at = $2, user_id = $3
		WHERE id = $4
	`, authCode, authCodeExpires, userID, session.ID)

	if err != nil {
		return "", "", err
	}

	return authCode, session.RedirectURI, nil
}

// ExchangeAuthCode exchanges an authorization code for an access token
func (s *MCPOAuthStore) ExchangeAuthCode(ctx context.Context, authCode, codeVerifier string) (*MCPAccessTokenWithSecret, error) {
	// Find the session by auth code
	var session MCPOAuthSession
	err := s.pool.QueryRow(ctx, `
		SELECT id, state, code_challenge, code_challenge_method, redirect_uri,
		       auth_code, auth_code_expires_at, user_id, created_at, expires_at
		FROM mcp_oauth_sessions
		WHERE auth_code = $1
	`, authCode).Scan(
		&session.ID, &session.State, &session.CodeChallenge, &session.CodeChallengeMethod,
		&session.RedirectURI, &session.AuthCode, &session.AuthCodeExpiresAt,
		&session.UserID, &session.CreatedAt, &session.ExpiresAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidAuthCode
		}
		return nil, err
	}

	// Check expiration
	if session.AuthCodeExpiresAt == nil || time.Now().After(*session.AuthCodeExpiresAt) {
		return nil, ErrAuthCodeExpired
	}

	// Verify PKCE
	if !verifyCodeChallenge(codeVerifier, session.CodeChallenge, session.CodeChallengeMethod) {
		return nil, ErrCodeChallengeInvalid
	}

	if session.UserID == nil {
		return nil, ErrInvalidAuthCode
	}

	// Delete the session (auth codes are single-use)
	_, _ = s.pool.Exec(ctx, `DELETE FROM mcp_oauth_sessions WHERE id = $1`, session.ID)

	// Generate access token
	token, prefix, hash, err := generateMCPToken()
	if err != nil {
		return nil, err
	}

	accessToken := &MCPAccessTokenWithSecret{
		MCPAccessToken: MCPAccessToken{
			ID:          uuid.New(),
			UserID:      *session.UserID,
			TokenHash:   hash,
			TokenPrefix: prefix,
			ExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
			CreatedAt:   time.Now().UTC(),
		},
		Token: token,
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO mcp_access_tokens (id, user_id, token_hash, token_prefix, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, accessToken.ID, accessToken.UserID, hash, prefix, accessToken.ExpiresAt, accessToken.CreatedAt)

	if err != nil {
		return nil, err
	}

	return accessToken, nil
}

// ValidateToken checks if an MCP access token is valid and returns the user ID
func (s *MCPOAuthStore) ValidateToken(ctx context.Context, token string) (uuid.UUID, error) {
	hash := hashToken(token)

	var userID uuid.UUID
	var tokenID uuid.UUID
	var expiresAt time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, expires_at FROM mcp_access_tokens WHERE token_hash = $1
	`, hash).Scan(&tokenID, &userID, &expiresAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrMCPTokenNotFound
		}
		return uuid.Nil, err
	}

	if time.Now().After(expiresAt) {
		return uuid.Nil, ErrMCPTokenExpired
	}

	// Update last_used_at asynchronously
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = s.pool.Exec(ctx, `
			UPDATE mcp_access_tokens SET last_used_at = NOW() WHERE id = $1
		`, tokenID)
	}()

	return userID, nil
}

// CleanupExpired removes expired sessions and tokens
func (s *MCPOAuthStore) CleanupExpired(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM mcp_oauth_sessions WHERE expires_at < NOW()
	`)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		DELETE FROM mcp_access_tokens WHERE expires_at < NOW()
	`)
	return err
}
