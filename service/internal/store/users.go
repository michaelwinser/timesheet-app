package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrEmailAlreadyTaken = errors.New("email already registered")
	ErrInvalidPassword   = errors.New("invalid password")
)

// User represents a stored user
type User struct {
	ID           openapi_types.UUID
	Email        openapi_types.Email
	Name         string
	PasswordHash string
	CreatedAt    time.Time
}

// UserStore provides PostgreSQL-backed user storage
type UserStore struct {
	pool *pgxpool.Pool
}

// NewUserStore creates a new PostgreSQL user store
func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

// Create adds a new user with the given email, name, and password
func (s *UserStore) Create(ctx context.Context, email, name, password string) (*User, error) {
	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{
		ID:           uuid.New(),
		Email:        openapi_types.Email(email),
		Name:         name,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
	`, user.ID, email, name, user.PasswordHash, user.CreatedAt)

	if err != nil {
		// Check for unique constraint violation
		if isDuplicateKeyError(err) {
			return nil, ErrEmailAlreadyTaken
		}
		return nil, err
	}

	return user, nil
}

// GetByID retrieves a user by ID
func (s *UserStore) GetByID(ctx context.Context, id openapi_types.UUID) (*User, error) {
	user := &User{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, created_at
		FROM users WHERE id = $1
	`, id).Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// GetByEmail retrieves a user by email
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	user := &User{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, created_at
		FROM users WHERE email = $1
	`, email).Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// Authenticate checks email/password and returns the user if valid
func (s *UserStore) Authenticate(ctx context.Context, email, password string) (*User, error) {
	user, err := s.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidPassword // Don't reveal whether email exists
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	return user, nil
}

// isDuplicateKeyError checks if the error is a PostgreSQL unique constraint violation
func isDuplicateKeyError(err error) bool {
	// PostgreSQL error code 23505 is unique_violation
	return err != nil && (contains(err.Error(), "23505") || contains(err.Error(), "duplicate key"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
