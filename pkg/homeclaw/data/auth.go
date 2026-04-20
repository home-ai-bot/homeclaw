// Package data provides data access layer for HomeClaw.
package data

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// encryptionKey is the fixed key used for password encryption
// In production, you may want to generate this from device-specific info
const encryptionKey = "homeclaw-auth-key-v1"

// BrandAuthData stores credentials for a single brand
type BrandAuthData struct {
	Brand    string            `json:"brand"`               // "tuya", "xiaomi", etc.
	Region   string            `json:"region,omitempty"`    // Region (brand-specific)
	UserName string            `json:"user_name,omitempty"` // Username/email
	Token    string            `json:"token"`               // Encrypted token/password
	Extra    map[string]string `json:"extra,omitempty"`     // Brand-specific fields
}

// AuthData is the root structure for auth.json
type AuthData struct {
	Version string                    `json:"version"`
	Brands  map[string]*BrandAuthData `json:"brands"` // Key is brand name
}

// AuthStore defines the interface for brand authentication operations
type AuthStore interface {
	GetBrand(brand string) (*BrandAuthData, error)
	SaveBrand(brand, region, userName, token string, extra map[string]string) error
	GetDecryptedBrand(brand string) (region, userName, token string, extra map[string]string, err error)
	DeleteBrand(brand string) error
	ListBrands() ([]string, error)
	Exists(brand string) bool
}

// authStore implements AuthStore using JSONStore
type authStore struct {
	store *JSONStore
	data  AuthData
}

// ErrBrandNotFound is returned when brand auth data is not found
var ErrBrandNotFound = errors.New("auth: brand credentials not found")

// NewAuthStore creates a new AuthStore
func NewAuthStore(store *JSONStore) (AuthStore, error) {
	s := &authStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads auth data from file
func (s *authStore) load() error {
	s.data = AuthData{
		Version: "1",
		Brands:  make(map[string]*BrandAuthData),
	}
	return s.store.Read("auth", &s.data)
}

// save writes auth data to file
func (s *authStore) save() error {
	return s.store.Write("auth", s.data)
}

// GetBrand returns the stored auth data for a brand (token is encrypted)
func (s *authStore) GetBrand(brand string) (*BrandAuthData, error) {
	authData, ok := s.data.Brands[brand]
	if !ok || authData.Token == "" {
		return nil, ErrBrandNotFound
	}
	return authData, nil
}

// SaveBrand stores the credentials for a brand (token encrypted)
func (s *authStore) SaveBrand(brand, region, userName, token string, extra map[string]string) error {
	// Encrypt the token
	encryptedToken, err := encrypt(token)
	if err != nil {
		return fmt.Errorf("auth: failed to encrypt token: %w", err)
	}

	// Create or update brand data
	s.data.Brands[brand] = &BrandAuthData{
		Brand:    brand,
		Region:   region,
		UserName: userName,
		Token:    encryptedToken,
		Extra:    extra,
	}

	return s.save()
}

// GetDecryptedBrand returns the decrypted credentials for a brand
func (s *authStore) GetDecryptedBrand(brand string) (region, userName, token string, extra map[string]string, err error) {
	authData, ok := s.data.Brands[brand]
	if !ok || authData.Token == "" {
		return "", "", "", nil, ErrBrandNotFound
	}

	// Decrypt the token
	decryptedToken, err := decrypt(authData.Token)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("auth: failed to decrypt token: %w", err)
	}

	return authData.Region, authData.UserName, decryptedToken, authData.Extra, nil
}

// DeleteBrand removes the stored credentials for a brand
func (s *authStore) DeleteBrand(brand string) error {
	delete(s.data.Brands, brand)
	return s.save()
}

// ListBrands returns all brand names that have stored credentials
func (s *authStore) ListBrands() ([]string, error) {
	var brands []string
	for brand, authData := range s.data.Brands {
		if authData.Token != "" {
			brands = append(brands, brand)
		}
	}
	return brands, nil
}

// Exists checks if credentials are stored for a brand
func (s *authStore) Exists(brand string) bool {
	authData, ok := s.data.Brands[brand]
	return ok && authData.Token != ""
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
