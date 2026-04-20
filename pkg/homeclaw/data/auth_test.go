package data

import (
	"os"
	"path/filepath"
	"testing"
)

func setupAuthTest(t *testing.T) (AuthStore, func()) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewJSONStore(dir)
	if err != nil {
		t.Fatalf("failed to create JSONStore: %v", err)
	}

	authStore, err := NewAuthStore(store)
	if err != nil {
		t.Fatalf("failed to create AuthStore: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(dir)
	}

	return authStore, cleanup
}

func TestAuthStore_SaveAndGetBrand(t *testing.T) {
	store, cleanup := setupAuthTest(t)
	defer cleanup()

	// Save Tuya credentials
	err := store.SaveBrand("tuya", "US", "user@example.com", "secret-token-123", nil)
	if err != nil {
		t.Fatalf("SaveBrand failed: %v", err)
	}

	// Verify it exists
	if !store.Exists("tuya") {
		t.Error("Expected tuya brand to exist")
	}

	// Get the encrypted data
	authData, err := store.GetBrand("tuya")
	if err != nil {
		t.Fatalf("GetBrand failed: %v", err)
	}

	if authData.Brand != "tuya" {
		t.Errorf("Expected brand 'tuya', got '%s'", authData.Brand)
	}
	if authData.Region != "US" {
		t.Errorf("Expected region 'US', got '%s'", authData.Region)
	}
	if authData.UserName != "user@example.com" {
		t.Errorf("Expected username 'user@example.com', got '%s'", authData.UserName)
	}
	if authData.Token == "" {
		t.Error("Expected token to be encrypted, got empty string")
	}

	// Get decrypted data
	region, userName, token, extra, err := store.GetDecryptedBrand("tuya")
	if err != nil {
		t.Fatalf("GetDecryptedBrand failed: %v", err)
	}

	if region != "US" {
		t.Errorf("Expected decrypted region 'US', got '%s'", region)
	}
	if userName != "user@example.com" {
		t.Errorf("Expected decrypted username 'user@example.com', got '%s'", userName)
	}
	if token != "secret-token-123" {
		t.Errorf("Expected decrypted token 'secret-token-123', got '%s'", token)
	}
	if extra != nil {
		t.Error("Expected extra to be nil")
	}
}

func TestAuthStore_MultipleBrands(t *testing.T) {
	store, cleanup := setupAuthTest(t)
	defer cleanup()

	// Save Tuya credentials
	err := store.SaveBrand("tuya", "US", "tuya@example.com", "tuya-token", nil)
	if err != nil {
		t.Fatalf("SaveBrand tuya failed: %v", err)
	}

	// Save Xiaomi credentials
	extra := map[string]string{"device_type": "gateway"}
	err = store.SaveBrand("xiaomi", "CN", "xiaomi@example.com", "xiaomi-token", extra)
	if err != nil {
		t.Fatalf("SaveBrand xiaomi failed: %v", err)
	}

	// Verify both exist
	if !store.Exists("tuya") {
		t.Error("Expected tuya brand to exist")
	}
	if !store.Exists("xiaomi") {
		t.Error("Expected xiaomi brand to exist")
	}

	// List brands
	brands, err := store.ListBrands()
	if err != nil {
		t.Fatalf("ListBrands failed: %v", err)
	}

	if len(brands) != 2 {
		t.Errorf("Expected 2 brands, got %d", len(brands))
	}

	// Verify tuya data
	region, userName, token, _, err := store.GetDecryptedBrand("tuya")
	if err != nil {
		t.Fatalf("GetDecryptedBrand tuya failed: %v", err)
	}
	if region != "US" || userName != "tuya@example.com" || token != "tuya-token" {
		t.Errorf("Tuya data mismatch: region=%s, userName=%s, token=%s", region, userName, token)
	}

	// Verify xiaomi data
	region, userName, token, extraData, err := store.GetDecryptedBrand("xiaomi")
	if err != nil {
		t.Fatalf("GetDecryptedBrand xiaomi failed: %v", err)
	}
	if region != "CN" || userName != "xiaomi@example.com" || token != "xiaomi-token" {
		t.Errorf("Xiaomi data mismatch: region=%s, userName=%s, token=%s", region, userName, token)
	}
	if extraData["device_type"] != "gateway" {
		t.Errorf("Expected extra device_type 'gateway', got '%s'", extraData["device_type"])
	}
}

func TestAuthStore_DeleteBrand(t *testing.T) {
	store, cleanup := setupAuthTest(t)
	defer cleanup()

	// Save and verify
	err := store.SaveBrand("tuya", "US", "user@example.com", "token", nil)
	if err != nil {
		t.Fatalf("SaveBrand failed: %v", err)
	}

	if !store.Exists("tuya") {
		t.Fatal("Expected tuya to exist after save")
	}

	// Delete
	err = store.DeleteBrand("tuya")
	if err != nil {
		t.Fatalf("DeleteBrand failed: %v", err)
	}

	// Verify deletion
	if store.Exists("tuya") {
		t.Error("Expected tuya to not exist after deletion")
	}

	// Try to get deleted brand
	_, err = store.GetBrand("tuya")
	if err == nil {
		t.Error("Expected error when getting deleted brand")
	}
	if err != ErrBrandNotFound {
		t.Errorf("Expected ErrBrandNotFound, got %v", err)
	}
}

func TestAuthStore_GetDecryptedBrand_NotFound(t *testing.T) {
	store, cleanup := setupAuthTest(t)
	defer cleanup()

	_, _, _, _, err := store.GetDecryptedBrand("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent brand")
	}
	if err != ErrBrandNotFound {
		t.Errorf("Expected ErrBrandNotFound, got %v", err)
	}
}

func TestAuthStore_UpdateBrand(t *testing.T) {
	store, cleanup := setupAuthTest(t)
	defer cleanup()

	// Save initial credentials
	err := store.SaveBrand("tuya", "US", "old@example.com", "old-token", nil)
	if err != nil {
		t.Fatalf("SaveBrand failed: %v", err)
	}

	// Update credentials
	err = store.SaveBrand("tuya", "EU", "new@example.com", "new-token", nil)
	if err != nil {
		t.Fatalf("Update SaveBrand failed: %v", err)
	}

	// Verify updated data
	region, userName, token, _, err := store.GetDecryptedBrand("tuya")
	if err != nil {
		t.Fatalf("GetDecryptedBrand failed: %v", err)
	}

	if region != "EU" {
		t.Errorf("Expected updated region 'EU', got '%s'", region)
	}
	if userName != "new@example.com" {
		t.Errorf("Expected updated username 'new@example.com', got '%s'", userName)
	}
	if token != "new-token" {
		t.Errorf("Expected updated token 'new-token', got '%s'", token)
	}
}

func TestAuthStore_EncryptionSecurity(t *testing.T) {
	store, cleanup := setupAuthTest(t)
	defer cleanup()

	// Save a token
	originalToken := "super-secret-token-12345"
	err := store.SaveBrand("tuya", "US", "user@example.com", originalToken, nil)
	if err != nil {
		t.Fatalf("SaveBrand failed: %v", err)
	}

	// Get the encrypted data
	authData, err := store.GetBrand("tuya")
	if err != nil {
		t.Fatalf("GetBrand failed: %v", err)
	}

	// Verify the stored token is encrypted (not the same as original)
	if authData.Token == originalToken {
		t.Error("Token should be encrypted, but stored token matches original")
	}

	// Verify decryption returns the original
	_, _, decryptedToken, _, err := store.GetDecryptedBrand("tuya")
	if err != nil {
		t.Fatalf("GetDecryptedBrand failed: %v", err)
	}

	if decryptedToken != originalToken {
		t.Errorf("Decrypted token '%s' doesn't match original '%s'", decryptedToken, originalToken)
	}
}

func TestAuthStore_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Create first store and save data
	store1, err := NewJSONStore(dir)
	if err != nil {
		t.Fatalf("Failed to create JSONStore: %v", err)
	}

	authStore1, err := NewAuthStore(store1)
	if err != nil {
		t.Fatalf("Failed to create AuthStore: %v", err)
	}

	err = authStore1.SaveBrand("tuya", "US", "user@example.com", "persistent-token", nil)
	if err != nil {
		t.Fatalf("SaveBrand failed: %v", err)
	}

	// Create a new store instance (simulating restart)
	store2, err := NewJSONStore(dir)
	if err != nil {
		t.Fatalf("Failed to create second JSONStore: %v", err)
	}

	authStore2, err := NewAuthStore(store2)
	if err != nil {
		t.Fatalf("Failed to create second AuthStore: %v", err)
	}

	// Verify data persists
	if !authStore2.Exists("tuya") {
		t.Error("Expected tuya brand to persist across store instances")
	}

	region, userName, token, _, err := authStore2.GetDecryptedBrand("tuya")
	if err != nil {
		t.Fatalf("GetDecryptedBrand failed: %v", err)
	}

	if region != "US" || userName != "user@example.com" || token != "persistent-token" {
		t.Errorf("Persisted data mismatch: region=%s, userName=%s, token=%s", region, userName, token)
	}
}

func TestAuthStore_ListBrands(t *testing.T) {
	store, cleanup := setupAuthTest(t)
	defer cleanup()

	// Initially empty
	brands, err := store.ListBrands()
	if err != nil {
		t.Fatalf("ListBrands failed: %v", err)
	}
	if len(brands) != 0 {
		t.Errorf("Expected 0 brands initially, got %d", len(brands))
	}

	// Add brands
	store.SaveBrand("tuya", "US", "tuya@test.com", "token1", nil)
	store.SaveBrand("xiaomi", "CN", "xiaomi@test.com", "token2", nil)
	store.SaveBrand("hue", "EU", "hue@test.com", "token3", nil)

	// List again
	brands, err = store.ListBrands()
	if err != nil {
		t.Fatalf("ListBrands failed: %v", err)
	}

	if len(brands) != 3 {
		t.Errorf("Expected 3 brands, got %d: %v", len(brands), brands)
	}

	// Verify all brands are in the list
	brandMap := make(map[string]bool)
	for _, b := range brands {
		brandMap[b] = true
	}

	if !brandMap["tuya"] || !brandMap["xiaomi"] || !brandMap["hue"] {
		t.Errorf("Expected all brands in list, got %v", brands)
	}
}

func TestAuthStore_JSONFileCreated(t *testing.T) {
	dir := t.TempDir()

	store, err := NewJSONStore(dir)
	if err != nil {
		t.Fatalf("Failed to create JSONStore: %v", err)
	}

	authStore, err := NewAuthStore(store)
	if err != nil {
		t.Fatalf("Failed to create AuthStore: %v", err)
	}

	// Save data to trigger file creation
	err = authStore.SaveBrand("tuya", "US", "user@test.com", "token", nil)
	if err != nil {
		t.Fatalf("SaveBrand failed: %v", err)
	}

	// Verify auth.json file was created
	authFilePath := filepath.Join(dir, "auth.json")
	if _, err := os.Stat(authFilePath); os.IsNotExist(err) {
		t.Error("Expected auth.json file to be created")
	}
}
