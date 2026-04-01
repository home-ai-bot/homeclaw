package tuya

import (
	"testing"

	rootdata "github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

func TestNewClient(t *testing.T) {
	// Create temp directory for JSONStore
	tmpDir := t.TempDir()
	store, err := rootdata.NewJSONStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create client with credentials
	client, err := NewClient(store, WithCredentials("china", "17091616150", "xd1tXdks"))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Error("Expected client to be non-nil")
	}
	result, err := client.Login()
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if result == nil {
		t.Error("Expected login result to be non-nil")
	}
	//打印result
	t.Logf("Login result: %v", result)
}
