package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONStoreBackup(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	store, err := NewJSONStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Test write
	data := SpacesData{
		Version: "1",
		Spaces: []Space{
			{ID: "living-room", Name: "客厅", Type: "room"},
		},
	}
	if err := store.Write("spaces", data); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Verify backup file exists
	bakPath := filepath.Join(tmpDir, "spaces.json.bak")
	if _, err := os.Stat(bakPath); !os.IsNotExist(err) {
		t.Error("Backup should not exist after first write")
	}

	// Second write should create backup
	data.Spaces = append(data.Spaces, Space{ID: "kitchen", Name: "厨房", Type: "room"})
	if err := store.Write("spaces", data); err != nil {
		t.Fatalf("Failed to write second time: %v", err)
	}

	// Now backup should exist
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		t.Error("Backup should exist after second write")
	}

	// Test read
	var readData SpacesData
	if err := store.Read("spaces", &readData); err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	if len(readData.Spaces) != 2 {
		t.Errorf("Expected 2 spaces, got %d", len(readData.Spaces))
	}
}

func TestSpaceStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewJSONStore(tmpDir)
	spaceStore, err := NewSpaceStore(store)
	if err != nil {
		t.Fatalf("Failed to create space store: %v", err)
	}

	// Save space
	space := Space{ID: "test-room", Name: "测试房间", Type: "room"}
	if err := spaceStore.Save(space); err != nil {
		t.Fatalf("Failed to save space: %v", err)
	}

	// Get by ID
	found, err := spaceStore.GetByID("test-room")
	if err != nil {
		t.Fatalf("Failed to get space: %v", err)
	}
	if found.Name != "测试房间" {
		t.Errorf("Expected name '测试房间', got '%s'", found.Name)
	}

	// Find by name
	found2, err := spaceStore.FindByName("测试房间")
	if err != nil {
		t.Fatalf("Failed to find space: %v", err)
	}
	if found2.ID != "test-room" {
		t.Errorf("Expected ID 'test-room', got '%s'", found2.ID)
	}

	// Update
	space.Name = "新名字"
	if err := spaceStore.Save(space); err != nil {
		t.Fatalf("Failed to update space: %v", err)
	}
	found3, _ := spaceStore.GetByID("test-room")
	if found3.Name != "新名字" {
		t.Errorf("Expected updated name '新名字', got '%s'", found3.Name)
	}

	// Delete
	if err := spaceStore.Delete("test-room"); err != nil {
		t.Fatalf("Failed to delete space: %v", err)
	}
	if _, err := spaceStore.GetByID("test-room"); err != ErrRecordNotFound {
		t.Error("Expected ErrRecordNotFound after delete")
	}
}

func TestDeviceStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewJSONStore(tmpDir)
	deviceStore, err := NewDeviceStore(store)
	if err != nil {
		t.Fatalf("Failed to create device store: %v", err)
	}

	// Save device
	device := Device{
		ID:      "light-001",
		Name:    "客厅灯",
		Brand:   "mijia",
		SpaceID: "living-room",
	}
	if err := deviceStore.Save(device); err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	// Verify device was saved and retrievable by space
	devices, err := deviceStore.GetBySpace("living-room")
	if err != nil {
		t.Fatalf("Failed to get devices by space: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}
}

func TestMemberStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewJSONStore(tmpDir)
	memberStore, err := NewMemberStore(store)
	if err != nil {
		t.Fatalf("Failed to create member store: %v", err)
	}

	// Save member
	member := Member{
		Name:             "爸爸",
		Role:             "admin",
		SpacePermissions: []string{"*"},
		Channels: map[string]ChannelInfo{
			"telegram": {UserID: "123456", BoundAt: time.Now()},
		},
		CreatedAt: time.Now(),
	}
	if err := memberStore.Save(member); err != nil {
		t.Fatalf("Failed to save member: %v", err)
	}

	// Get by name
	found, err := memberStore.GetByName("爸爸")
	if err != nil {
		t.Fatalf("Failed to get member: %v", err)
	}
	if found.Role != "admin" {
		t.Errorf("Expected role 'admin', got '%s'", found.Role)
	}

	// Get by channel ID
	found2, err := memberStore.GetByChannelID("telegram", "123456")
	if err != nil {
		t.Fatalf("Failed to get member by channel: %v", err)
	}
	if found2.Name != "爸爸" {
		t.Errorf("Expected name '爸爸', got '%s'", found2.Name)
	}
}

func TestWorkflowStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewJSONStore(tmpDir)
	workflowStore, err := NewWorkflowStore(store)
	if err != nil {
		t.Fatalf("Failed to create workflow store: %v", err)
	}

	// Create workflow definition
	workflow := &WorkflowDef{
		ID:          "wf-test",
		Name:        "测试工作流",
		Description: "用于测试的工作流",
		Version:     "1",
		Triggers: []Trigger{
			{Type: "intent", Patterns: []string{"测试"}},
		},
		Context: WorkflowContext{
			Space:  "current",
			Member: "current",
		},
		Steps: []Step{
			{
				ID:     "step1",
				Type:   StepTypeAction,
				Name:   "第一步",
				Action: "device.control",
				Params: map[string]interface{}{
					"device": "light",
					"power":  "on",
				},
				OutputAs: "result1",
			},
		},
	}

	// Save workflow
	if err := workflowStore.Save(workflow, "admin"); err != nil {
		t.Fatalf("Failed to save workflow: %v", err)
	}

	// Get metadata by ID
	meta, err := workflowStore.GetMetaByID("wf-test")
	if err != nil {
		t.Fatalf("Failed to get workflow meta: %v", err)
	}
	if meta.Name != "测试工作流" {
		t.Errorf("Expected name '测试工作流', got '%s'", meta.Name)
	}
	if !meta.Enabled {
		t.Error("Expected workflow to be enabled")
	}

	// Find by name
	meta2, err := workflowStore.FindMetaByName("测试工作流")
	if err != nil {
		t.Fatalf("Failed to find workflow: %v", err)
	}
	if meta2.ID != "wf-test" {
		t.Errorf("Expected ID 'wf-test', got '%s'", meta2.ID)
	}

	// Get full definition
	def, err := workflowStore.GetByID("wf-test")
	if err != nil {
		t.Fatalf("Failed to get workflow: %v", err)
	}
	if len(def.Steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(def.Steps))
	}

	// Disable workflow
	if err := workflowStore.Disable("wf-test"); err != nil {
		t.Fatalf("Failed to disable workflow: %v", err)
	}

	// Should fail to get disabled workflow
	_, err = workflowStore.GetByID("wf-test")
	if err == nil {
		t.Error("Expected error when getting disabled workflow")
	}

	// Enable workflow
	if err := workflowStore.Enable("wf-test"); err != nil {
		t.Fatalf("Failed to enable workflow: %v", err)
	}

	// Should succeed now
	_, err = workflowStore.GetByID("wf-test")
	if err != nil {
		t.Errorf("Expected no error after enabling, got: %v", err)
	}

	// Update workflow
	workflow.Description = "更新后的描述"
	if err := workflowStore.Save(workflow, "admin"); err != nil {
		t.Fatalf("Failed to update workflow: %v", err)
	}

	// Verify update
	meta3, _ := workflowStore.GetMetaByID("wf-test")
	if meta3.Description != "更新后的描述" {
		t.Errorf("Expected updated description, got '%s'", meta3.Description)
	}

	// Delete workflow
	if err := workflowStore.Delete("wf-test"); err != nil {
		t.Fatalf("Failed to delete workflow: %v", err)
	}

	// Verify deletion
	_, err = workflowStore.GetMetaByID("wf-test")
	if err != ErrRecordNotFound {
		t.Error("Expected ErrRecordNotFound after delete")
	}
}
