package data

import (
	"os"
	"path/filepath"
	"testing"
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
			{Name: "客厅", From: map[string]string{"name": "manual"}},
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
	data.Spaces = append(data.Spaces, Space{Name: "厨房", From: map[string]string{"name": "manual"}})
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
	space := Space{Name: "测试房间", From: map[string]string{"name": "manual"}}
	if err := spaceStore.Save(space); err != nil {
		t.Fatalf("Failed to save space: %v", err)
	}

	// Get by name
	spaces, err := spaceStore.GetAll()
	if err != nil {
		t.Fatalf("Failed to get spaces: %v", err)
	}
	var found *Space
	for i := range spaces {
		if spaces[i].Name == "测试房间" {
			found = &spaces[i]
			break
		}
	}
	if found == nil {
		t.Fatal("Space not found")
	}
	if found.Name != "测试房间" {
		t.Errorf("Expected name '测试房间', got '%s'", found.Name)
	}

	// Find by name (second lookup)
	spaces2, _ := spaceStore.GetAll()
	var found2 *Space
	for i := range spaces2 {
		if spaces2[i].Name == "测试房间" {
			found2 = &spaces2[i]
			break
		}
	}
	if found2 == nil || found2.Name != "测试房间" {
		t.Errorf("Expected name '测试房间'")
	}

	// Update
	space.From = map[string]string{"name": "xiaomi"}
	if err := spaceStore.Save(space); err != nil {
		t.Fatalf("Failed to update space: %v", err)
	}
	spaces3, _ := spaceStore.GetAll()
	var found3 *Space
	for i := range spaces3 {
		if spaces3[i].Name == "测试房间" {
			found3 = &spaces3[i]
			break
		}
	}
	if found3 == nil || found3.From["name"] != "xiaomi" {
		t.Errorf("Expected updated from 'xiaomi'")
	}

	// Delete
	if err := spaceStore.Delete("测试房间"); err != nil {
		t.Fatalf("Failed to delete space: %v", err)
	}
	spaces4, _ := spaceStore.GetAll()
	deleted := true
	for _, s := range spaces4 {
		if s.Name == "测试房间" {
			deleted = false
			break
		}
	}
	if !deleted {
		t.Error("Expected space to be deleted")
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
		FromID:    "light-001",
		From:      "mijia",
		Name:      "客厅灯",
		SpaceName: "客厅",
	}
	if err := deviceStore.Save(device); err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	// Verify device was saved
	devices, err := deviceStore.GetAll()
	if err != nil {
		t.Fatalf("Failed to get devices: %v", err)
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
		Name:       "爸爸",
		Role:       "admin",
		MySpaces:   []string{"客厅", "书房"},
		SleepSpace: "主卧",
	}
	if err := memberStore.Save(member); err != nil {
		t.Fatalf("Failed to save member: %v", err)
	}

	// Get by name via GetAll
	members, err := memberStore.GetAll()
	if err != nil {
		t.Fatalf("Failed to get members: %v", err)
	}
	var found *Member
	for i := range members {
		if members[i].Name == "爸爸" {
			found = &members[i]
			break
		}
	}
	if found == nil {
		t.Fatal("Member not found")
	}
	if found.Role != "admin" {
		t.Errorf("Expected role 'admin', got '%s'", found.Role)
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
