package store

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrEmailAlreadyTaken = errors.New("email already registered")
	ErrInvalidPassword   = errors.New("invalid password")
)

// User represents a stored user with password hash
type User struct {
	ID           openapi_types.UUID
	Email        openapi_types.Email
	Name         string
	PasswordHash string
	CreatedAt    time.Time
}

// UserStore provides in-memory user storage
type UserStore struct {
	mu       sync.RWMutex
	users    map[openapi_types.UUID]*User
	byEmail  map[string]*User
}

// NewUserStore creates a new in-memory user store
func NewUserStore() *UserStore {
	return &UserStore{
		users:   make(map[openapi_types.UUID]*User),
		byEmail: make(map[string]*User),
	}
}

// Create adds a new user with the given email, name, and password
func (s *UserStore) Create(email, name, password string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if email is already taken
	if _, exists := s.byEmail[email]; exists {
		return nil, ErrEmailAlreadyTaken
	}

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

	s.users[user.ID] = user
	s.byEmail[email] = user

	return user, nil
}

// GetByID retrieves a user by ID
func (s *UserStore) GetByID(id openapi_types.UUID) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// GetByEmail retrieves a user by email
func (s *UserStore) GetByEmail(email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.byEmail[email]
	if !ok {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// Authenticate checks email/password and returns the user if valid
func (s *UserStore) Authenticate(email, password string) (*User, error) {
	user, err := s.GetByEmail(email)
	if err != nil {
		return nil, ErrInvalidPassword // Don't reveal whether email exists
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	return user, nil
}
