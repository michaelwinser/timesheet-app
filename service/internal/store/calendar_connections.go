package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/michaelw/timesheet-app/service/internal/crypto"
)

var (
	ErrCalendarConnectionNotFound = errors.New("calendar connection not found")
	ErrCalendarAlreadyConnected   = errors.New("calendar already connected")
)

// OAuthCredentials stores OAuth token data
type OAuthCredentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

// CalendarConnection represents a linked calendar
type CalendarConnection struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Provider     string
	Credentials  OAuthCredentials // Decrypted
	SyncToken    *string          // Google Calendar sync token for incremental sync
	LastSyncedAt *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CalendarConnectionStore provides PostgreSQL-backed calendar connection storage
type CalendarConnectionStore struct {
	pool   *pgxpool.Pool
	crypto *crypto.EncryptionService
}

// NewCalendarConnectionStore creates a new store
func NewCalendarConnectionStore(pool *pgxpool.Pool, cryptoSvc *crypto.EncryptionService) *CalendarConnectionStore {
	return &CalendarConnectionStore{pool: pool, crypto: cryptoSvc}
}

// Create adds a new calendar connection with encrypted credentials
func (s *CalendarConnectionStore) Create(ctx context.Context, userID uuid.UUID, provider string, creds OAuthCredentials) (*CalendarConnection, error) {
	// Serialize and encrypt credentials
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}

	encrypted, err := s.crypto.Encrypt(credsJSON)
	if err != nil {
		return nil, err
	}

	conn := &CalendarConnection{
		ID:          uuid.New(),
		UserID:      userID,
		Provider:    provider,
		Credentials: creds,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO calendar_connections (id, user_id, provider, credentials_encrypted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, conn.ID, conn.UserID, provider, encrypted, conn.CreatedAt, conn.UpdatedAt)

	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, ErrCalendarAlreadyConnected
		}
		return nil, err
	}

	return conn, nil
}

// GetByID retrieves a connection by ID for a user (with decrypted credentials)
func (s *CalendarConnectionStore) GetByID(ctx context.Context, userID, connID uuid.UUID) (*CalendarConnection, error) {
	var encrypted []byte
	conn := &CalendarConnection{}

	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, provider, credentials_encrypted, sync_token, last_synced_at, created_at, updated_at
		FROM calendar_connections WHERE id = $1 AND user_id = $2
	`, connID, userID).Scan(
		&conn.ID, &conn.UserID, &conn.Provider, &encrypted,
		&conn.SyncToken, &conn.LastSyncedAt, &conn.CreatedAt, &conn.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarConnectionNotFound
		}
		return nil, err
	}

	// Decrypt credentials
	decrypted, err := s.crypto.Decrypt(encrypted)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(decrypted, &conn.Credentials); err != nil {
		return nil, err
	}

	return conn, nil
}

// List returns all connections for a user (without credentials for safety)
func (s *CalendarConnectionStore) List(ctx context.Context, userID uuid.UUID) ([]*CalendarConnection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, provider, last_synced_at, created_at, updated_at
		FROM calendar_connections WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connections []*CalendarConnection
	for rows.Next() {
		conn := &CalendarConnection{}
		err := rows.Scan(
			&conn.ID, &conn.UserID, &conn.Provider,
			&conn.LastSyncedAt, &conn.CreatedAt, &conn.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		connections = append(connections, conn)
	}

	return connections, rows.Err()
}

// UpdateCredentials updates the encrypted credentials (for token refresh)
func (s *CalendarConnectionStore) UpdateCredentials(ctx context.Context, connID uuid.UUID, creds OAuthCredentials) error {
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return err
	}

	encrypted, err := s.crypto.Encrypt(credsJSON)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE calendar_connections
		SET credentials_encrypted = $2, updated_at = $3
		WHERE id = $1
	`, connID, encrypted, time.Now().UTC())

	return err
}

// UpdateLastSynced updates the last_synced_at timestamp
func (s *CalendarConnectionStore) UpdateLastSynced(ctx context.Context, connID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE calendar_connections
		SET last_synced_at = $2, updated_at = $2
		WHERE id = $1
	`, connID, now)
	return err
}

// UpdateSyncToken updates the sync token for incremental sync
func (s *CalendarConnectionStore) UpdateSyncToken(ctx context.Context, connID uuid.UUID, token string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendar_connections
		SET sync_token = $2, updated_at = $3
		WHERE id = $1
	`, connID, token, time.Now().UTC())
	return err
}

// ClearSyncToken clears the sync token (forces full re-sync)
func (s *CalendarConnectionStore) ClearSyncToken(ctx context.Context, connID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendar_connections
		SET sync_token = NULL, updated_at = $2
		WHERE id = $1
	`, connID, time.Now().UTC())
	return err
}

// Delete removes a calendar connection
func (s *CalendarConnectionStore) Delete(ctx context.Context, userID, connID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		"DELETE FROM calendar_connections WHERE id = $1 AND user_id = $2",
		connID, userID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrCalendarConnectionNotFound
	}

	return nil
}

// GetByIDForSync retrieves a connection by ID (with decrypted credentials) for background sync.
// Unlike GetByID, this doesn't require a userID since background sync operates across all users.
func (s *CalendarConnectionStore) GetByIDForSync(ctx context.Context, connID uuid.UUID) (*CalendarConnection, error) {
	var encrypted []byte
	conn := &CalendarConnection{}

	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, provider, credentials_encrypted, sync_token, last_synced_at, created_at, updated_at
		FROM calendar_connections WHERE id = $1
	`, connID).Scan(
		&conn.ID, &conn.UserID, &conn.Provider, &encrypted,
		&conn.SyncToken, &conn.LastSyncedAt, &conn.CreatedAt, &conn.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarConnectionNotFound
		}
		return nil, err
	}

	// Decrypt credentials
	decrypted, err := s.crypto.Decrypt(encrypted)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(decrypted, &conn.Credentials); err != nil {
		return nil, err
	}

	return conn, nil
}
