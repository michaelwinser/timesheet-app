package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAPIKeyNotFound     = errors.New("API key not found")
	ErrAPIKeyNameTaken    = errors.New("API key name already exists")
	ErrInvalidAPIKey      = errors.New("invalid API key")
)

// APIKey represents a stored API key (without the actual key value)
type APIKey struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	KeyPrefix  string // First 8 chars for display
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// APIKeyWithSecret is returned only on creation, includes the raw key
type APIKeyWithSecret struct {
	APIKey
	Key string // The actual API key (only available at creation)
}

// APIKeyStore provides PostgreSQL-backed API key storage
type APIKeyStore struct {
	pool *pgxpool.Pool
}

// NewAPIKeyStore creates a new PostgreSQL API key store
func NewAPIKeyStore(pool *pgxpool.Pool) *APIKeyStore {
	return &APIKeyStore{pool: pool}
}

// generateKey creates a new random API key with prefix
// Format: ts_<32 random hex chars> (total 35 chars)
func generateKey() (key string, prefix string, hash string, err error) {
	// Generate 32 random bytes (256 bits)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", "", err
	}

	// Create the key with prefix
	key = "ts_" + hex.EncodeToString(randomBytes)
	prefix = key[:11] // "ts_" + first 8 hex chars

	// Hash the key for storage
	hashBytes := sha256.Sum256([]byte(key))
	hash = hex.EncodeToString(hashBytes[:])

	return key, prefix, hash, nil
}

// hashKey computes the SHA-256 hash of an API key
func hashKey(key string) string {
	hashBytes := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hashBytes[:])
}

// Create generates a new API key for a user
func (s *APIKeyStore) Create(ctx context.Context, userID uuid.UUID, name string) (*APIKeyWithSecret, error) {
	key, prefix, hash, err := generateKey()
	if err != nil {
		return nil, err
	}

	apiKey := &APIKeyWithSecret{
		APIKey: APIKey{
			ID:        uuid.New(),
			UserID:    userID,
			Name:      name,
			KeyPrefix: prefix,
			CreatedAt: time.Now().UTC(),
		},
		Key: key,
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, apiKey.ID, userID, name, hash, prefix, apiKey.CreatedAt)

	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, ErrAPIKeyNameTaken
		}
		return nil, err
	}

	return apiKey, nil
}

// List returns all API keys for a user (without the actual key values)
func (s *APIKeyStore) List(ctx context.Context, userID uuid.UUID) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, key_prefix, last_used_at, created_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.LastUsedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}

	return keys, rows.Err()
}

// Delete removes an API key
func (s *APIKeyStore) Delete(ctx context.Context, userID uuid.UUID, keyID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `
		DELETE FROM api_keys WHERE id = $1 AND user_id = $2
	`, keyID, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// ValidateAndGetUserID checks if an API key is valid and returns the associated user ID
func (s *APIKeyStore) ValidateAndGetUserID(ctx context.Context, key string) (uuid.UUID, error) {
	hash := hashKey(key)

	var userID uuid.UUID
	var keyID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id FROM api_keys WHERE key_hash = $1
	`, hash).Scan(&keyID, &userID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrInvalidAPIKey
		}
		return uuid.Nil, err
	}

	// Update last_used_at asynchronously (fire and forget)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = s.pool.Exec(ctx, `
			UPDATE api_keys SET last_used_at = NOW() WHERE id = $1
		`, keyID)
	}()

	return userID, nil
}
