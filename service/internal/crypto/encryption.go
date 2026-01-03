package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

var (
	ErrInvalidKey       = errors.New("encryption key must be 32 bytes (64 hex chars)")
	ErrDecryptionFailed = errors.New("decryption failed")
)

// EncryptionService provides AES-256-GCM encryption for sensitive data
type EncryptionService struct {
	gcm cipher.AEAD
}

// NewEncryptionService creates a new encryption service from a hex-encoded 32-byte key
func NewEncryptionService(keyHex string) (*EncryptionService, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &EncryptionService{gcm: gcm}, nil
}

// Encrypt encrypts plaintext and returns ciphertext with nonce prepended
func (s *EncryptionService) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return s.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts ciphertext (expects nonce prepended)
func (s *EncryptionService) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < s.gcm.NonceSize() {
		return nil, ErrDecryptionFailed
	}

	nonce := ciphertext[:s.gcm.NonceSize()]
	ciphertext = ciphertext[s.gcm.NonceSize():]

	plaintext, err := s.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}
