package tuya

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	rootdata "github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// encryptionKey is the fixed key used for password encryption
// In production, you may want to generate this from device-specific info
const encryptionKey = "tuya-secret-key-v1"

// SecretData represents the stored login credentials

type SecretData struct {
	Version  string `json:"version"`
	Region   string `json:"region"`
	UserName string `json:"user_name"`
	Password string `json:"password"` // encrypted password (base64)
}

// SecretStore defines the interface for secret data operations
type SecretStore interface {
	Get() (*SecretData, error)
	Save(region, userName, password string) error
	GetDecrypted() (region, userName, password string, err error)
	Delete() error
	Exists() bool
}

// secretStore implements SecretStore using JSONStore
type secretStore struct {
	store *rootdata.JSONStore
	data  SecretData
}

// ErrSecretNotFound is returned when secret data is not found
var ErrSecretNotFound = errors.New("secret: no credentials stored")

// NewSecretStore creates a new SecretStore
func NewSecretStore(store *rootdata.JSONStore) (SecretStore, error) {
	s := &secretStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads data from file
func (s *secretStore) load() error {
	s.data = SecretData{Version: "1"}
	return s.store.Read("tuya-secret", &s.data)
}

// save writes data to file
func (s *secretStore) save() error {
	return s.store.Write("tuya-secret", s.data)
}

// Get returns the stored secret data (password is encrypted)
func (s *secretStore) Get() (*SecretData, error) {
	if s.data.UserName == "" {
		return nil, ErrSecretNotFound
	}
	return &s.data, nil
}

// Save stores the credentials with encrypted password
func (s *secretStore) Save(region, userName, password string) error {
	// Encrypt the password with fixed key
	encryptedPwd, err := encrypt(password)
	if err != nil {
		return fmt.Errorf("secret: failed to encrypt password: %w", err)
	}

	s.data.Region = region
	s.data.UserName = userName
	s.data.Password = encryptedPwd

	return s.save()
}

// GetDecrypted returns the decrypted credentials
func (s *secretStore) GetDecrypted() (region, userName, password string, err error) {
	if s.data.UserName == "" {
		return "", "", "", ErrSecretNotFound
	}

	// Decrypt the password
	decryptedPwd, err := decrypt(s.data.Password)
	if err != nil {
		return "", "", "", fmt.Errorf("secret: failed to decrypt password: %w", err)
	}

	return s.data.Region, s.data.UserName, decryptedPwd, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Simple AES-GCM encryption with fixed key
// ────────────────────────────────────────────────────────────────────────────────

// deriveKey derives a 32-byte key from the fixed string
func deriveKey(key string) []byte {
	hash := sha256.Sum256([]byte(key))
	return hash[:]
}

// encrypt encrypts plaintext using AES-GCM with the fixed key
func encrypt(plaintext string) (string, error) {
	key := deriveKey(encryptionKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts ciphertext using AES-GCM with the fixed key
func decrypt(ciphertext string) (string, error) {
	key := deriveKey(encryptionKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}

	nonce := data[:gcm.NonceSize()]
	cipherData := data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// Delete removes the stored credentials
func (s *secretStore) Delete() error {
	s.data = SecretData{Version: "1"}
	return s.store.Remove("tuya-secret")
}

// Exists checks if credentials are stored
func (s *secretStore) Exists() bool {
	return s.data.UserName != ""
}
