package miio

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Unit tests for pure helpers (no network required)
// ─────────────────────────────────────────────────────────────────────────────

func TestGenerateNonce(t *testing.T) {
	n1 := generateNonce(time.Now().UnixMilli())
	if n1 == "" {
		t.Fatal("generateNonce returned empty string")
	}
	// Must be valid base64.
	if _, err := base64.StdEncoding.DecodeString(n1); err != nil {
		t.Fatalf("generateNonce result is not valid base64: %v", err)
	}
	decoded, _ := base64.StdEncoding.DecodeString(n1)
	if len(decoded) != 12 {
		t.Fatalf("expected 12-byte nonce, got %d bytes", len(decoded))
	}
	t.Logf("nonce: %s", n1)
}

func TestSignNonce(t *testing.T) {
	ssecurity := base64.StdEncoding.EncodeToString([]byte("test_ssecurity_16b"))
	nonce := generateNonce(time.Now().UnixMilli())
	signed := signNonce(ssecurity, nonce)
	if signed == "" {
		t.Fatal("signNonce returned empty string")
	}
	if _, err := base64.StdEncoding.DecodeString(signed); err != nil {
		t.Fatalf("signNonce result is not valid base64: %v", err)
	}
	// Deterministic: same inputs produce same output.
	signed2 := signNonce(ssecurity, nonce)
	if signed != signed2 {
		t.Fatalf("signNonce is not deterministic: %q != %q", signed, signed2)
	}
	t.Logf("signedNonce: %s", signed)
}

func TestApiHost(t *testing.T) {
	cases := []struct {
		country  string
		expected string
	}{
		{"cn", "https://api.io.mi.com/app"},
		{"us", "https://us.api.io.mi.com/app"},
		{"de", "https://de.api.io.mi.com/app"},
		{"sg", "https://sg.api.io.mi.com/app"},
		{"ru", "https://ru.api.io.mi.com/app"},
		{"tw", "https://tw.api.io.mi.com/app"},
		{"i2", "https://i2.api.io.mi.com/app"},
		{"", miAPIBase},
	}
	for _, tc := range cases {
		c := &CloudClient{Country: tc.country}
		got := c.apiHost()
		if got != tc.expected {
			t.Errorf("country=%q: expected %q, got %q", tc.country, tc.expected, got)
		}
	}
}

func TestNewCloudClient_DefaultCountry(t *testing.T) {
	c := NewCloudClient("")
	if c.Country != "cn" {
		t.Errorf("expected default country 'cn', got %q", c.Country)
	}
}

func TestExportImportSession(t *testing.T) {
	c := NewCloudClient("cn")
	c.UserID = "123456"
	c.SSECURITY = "abc"
	c.ServiceToken = "token_xyz"

	sess := c.ExportSession()
	if sess.UserID != "123456" || sess.SSECURITY != "abc" || sess.ServiceToken != "token_xyz" {
		t.Fatalf("ExportSession mismatch: %+v", sess)
	}

	c2 := NewCloudClient("us")
	c2.ImportSession(sess)
	if c2.UserID != "123456" || c2.SSECURITY != "abc" || c2.ServiceToken != "token_xyz" {
		t.Fatalf("ImportSession mismatch: %+v", c2)
	}
	// Country should be overridden by imported session.
	if c2.Country != "cn" {
		t.Errorf("expected country 'cn' after import, got %q", c2.Country)
	}
}

func TestImportSession_EmptyCountryPreservesExisting(t *testing.T) {
	c := NewCloudClient("us")
	c.ImportSession(Session{UserID: "u", SSECURITY: "s", ServiceToken: "t", Country: ""})
	// Empty country in session should not override existing.
	if c.Country != "us" {
		t.Errorf("expected country to remain 'us', got %q", c.Country)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration test – requires real Mi Home credentials and network access.
// Run with: go test -v -run TestLogin_Real ./pkg/homeclaw/miio/
// ─────────────────────────────────────────────────────────────────────────────

const (
	testUsername = "17091616150"
	testPassword = "52111125lili"
	testCountry  = "cn"
)

func TestLogin_Real(t *testing.T) {
	client := NewCloudClient(testCountry)

	t.Log("Step 1: logging in to Mi Home cloud...")
	if err := client.Login(testUsername, testPassword); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if client.UserID == "" {
		t.Error("UserID is empty after login")
	}
	if client.SSECURITY == "" {
		t.Error("SSECURITY is empty after login")
	}
	if client.ServiceToken == "" {
		t.Error("ServiceToken is empty after login")
	}
	t.Logf("Login OK — UserID=%s ServiceToken=%s...", client.UserID, client.ServiceToken[:min(16, len(client.ServiceToken))])

	t.Log("Step 2: fetching device list...")
	devices, err := client.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices failed: %v", err)
	}

	t.Logf("Found %d device(s):", len(devices))
	for i, d := range devices {
		t.Logf("  [%d] did=%-20s name=%-30s model=%-30s ip=%-16s token=%s",
			i+1, d.Did, d.Name, d.Model, d.IP, maskToken(d.Token))
	}

	if len(devices) == 0 {
		t.Log("WARN: device list is empty; verify the account has bound devices")
	}

	// Verify session export/import round-trip.
	sess := client.ExportSession()
	c2 := NewCloudClient(testCountry)
	c2.ImportSession(sess)
	devices2, err := c2.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices with imported session failed: %v", err)
	}
	if len(devices2) != len(devices) {
		t.Errorf("device count mismatch after session import: %d != %d", len(devices2), len(devices))
	}
	t.Log("Session export/import round-trip: OK")
}

// maskToken shows only the first 8 chars of a token for log safety.
func maskToken(token string) string {
	if len(token) <= 8 {
		return token
	}
	return fmt.Sprintf("%s...(%d)", token[:8], len(token))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
