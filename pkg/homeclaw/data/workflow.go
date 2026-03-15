// Package data provides data access layer for HomeClaw.
package data

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// WorkflowStore defines the interface for workflow data operations
type WorkflowStore interface {
	// Index operations
	GetAllMeta() ([]WorkflowMeta, error)
	GetMetaByID(id string) (*WorkflowMeta, error)
	FindMetaByName(name string) (*WorkflowMeta, error)

	// Workflow definition operations
	GetByID(id string) (*WorkflowDef, error)
	Save(def *WorkflowDef, createdBy string) error
	Delete(id string) error

	// Status management
	Enable(id string) error
	Disable(id string) error
	IsEnabled(id string) (bool, error)
}

// workflowStore implements WorkflowStore using JSONStore
type workflowStore struct {
	store        *JSONStore
	data         WorkflowsData
	workflowsDir string
}

// NewWorkflowStore creates a new WorkflowStore
func NewWorkflowStore(store *JSONStore) (WorkflowStore, error) {
	s := &workflowStore{
		store:        store,
		workflowsDir: "workflows",
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads index data from file
func (s *workflowStore) load() error {
	s.data = WorkflowsData{Version: "1", Workflows: []WorkflowMeta{}}
	return s.store.Read("workflow-index", &s.data)
}

// save writes index data to file
func (s *workflowStore) save() error {
	return s.store.Write("workflow-index", s.data)
}

// workflowFilePath returns the file path for a workflow definition
func (s *workflowStore) workflowFilePath(id string) string {
	return filepath.Join(s.workflowsDir, fmt.Sprintf("workflow-%s.json", id))
}

// GetAllMeta returns all workflow metadata
func (s *workflowStore) GetAllMeta() ([]WorkflowMeta, error) {
	return s.data.Workflows, nil
}

// GetMetaByID finds workflow metadata by ID
func (s *workflowStore) GetMetaByID(id string) (*WorkflowMeta, error) {
	for i := range s.data.Workflows {
		if s.data.Workflows[i].ID == id {
			return &s.data.Workflows[i], nil
		}
	}
	return nil, ErrRecordNotFound
}

// FindMetaByName finds workflow metadata by name (case-insensitive)
func (s *workflowStore) FindMetaByName(name string) (*WorkflowMeta, error) {
	lowerName := strings.ToLower(name)
	for i := range s.data.Workflows {
		if strings.ToLower(s.data.Workflows[i].Name) == lowerName {
			return &s.data.Workflows[i], nil
		}
	}
	return nil, ErrRecordNotFound
}

// GetByID loads a complete workflow definition by ID
func (s *workflowStore) GetByID(id string) (*WorkflowDef, error) {
	// First check metadata
	meta, err := s.GetMetaByID(id)
	if err != nil {
		return nil, err
	}

	// Check if enabled
	if !meta.Enabled {
		return nil, fmt.Errorf("workflow is disabled: %s", id)
	}

	// Load workflow definition from file
	var def WorkflowDef
	if err := s.store.Read(s.workflowFilePath(id), &def); err != nil {
		return nil, fmt.Errorf("failed to load workflow definition: %w", err)
	}

	return &def, nil
}

// Save saves a workflow (create or update)
func (s *workflowStore) Save(def *WorkflowDef, createdBy string) error {
	if def.ID == "" {
		return fmt.Errorf("workflow ID cannot be empty")
	}
	if def.Name == "" {
		return fmt.Errorf("workflow name cannot be empty")
	}

	now := time.Now()

	// Check if exists
	existing, err := s.GetMetaByID(def.ID)
	if err != nil && err != ErrRecordNotFound {
		return err
	}

	var meta WorkflowMeta
	if existing != nil {
		// Update existing
		meta = *existing
		meta.Name = def.Name
		meta.Description = def.Description
		meta.UpdatedAt = now
		def.UpdatedAt = now
	} else {
		// Create new
		meta = WorkflowMeta{
			ID:          def.ID,
			Name:        def.Name,
			Description: def.Description,
			FileName:    fmt.Sprintf("workflow-%s.json", def.ID),
			CreatedBy:   createdBy,
			CreatedAt:   now,
			UpdatedAt:   now,
			Enabled:     true,
		}
		def.CreatedAt = now
		def.UpdatedAt = now
		if def.CreatedBy == "" {
			def.CreatedBy = createdBy
		}
	}

	// Save workflow definition to file
	if err := s.store.Write(s.workflowFilePath(def.ID), def); err != nil {
		return fmt.Errorf("failed to save workflow definition: %w", err)
	}

	// Update index
	if existing != nil {
		// Update existing in slice
		for i := range s.data.Workflows {
			if s.data.Workflows[i].ID == def.ID {
				s.data.Workflows[i] = meta
				break
			}
		}
	} else {
		// Append new
		s.data.Workflows = append(s.data.Workflows, meta)
	}

	// Save index
	if err := s.save(); err != nil {
		return fmt.Errorf("failed to save workflow index: %w", err)
	}

	return nil
}

// Delete deletes a workflow by ID
func (s *workflowStore) Delete(id string) error {
	// Check if exists
	if _, err := s.GetMetaByID(id); err != nil {
		return err
	}

	// Remove from index
	for i := range s.data.Workflows {
		if s.data.Workflows[i].ID == id {
			s.data.Workflows = append(s.data.Workflows[:i], s.data.Workflows[i+1:]...)
			break
		}
	}

	// Save index
	if err := s.save(); err != nil {
		return fmt.Errorf("failed to save workflow index: %w", err)
	}

	// Delete workflow file
	if err := s.store.Remove(s.workflowFilePath(id)); err != nil {
		// Log but don't fail - index is already updated
		// The file can be cleaned up manually if needed
	}

	return nil
}

// Enable enables a workflow
func (s *workflowStore) Enable(id string) error {
	meta, err := s.GetMetaByID(id)
	if err != nil {
		return err
	}

	meta.Enabled = true
	meta.UpdatedAt = time.Now()

	// Update in slice
	for i := range s.data.Workflows {
		if s.data.Workflows[i].ID == id {
			s.data.Workflows[i] = *meta
			break
		}
	}

	return s.save()
}

// Disable disables a workflow
func (s *workflowStore) Disable(id string) error {
	meta, err := s.GetMetaByID(id)
	if err != nil {
		return err
	}

	meta.Enabled = false
	meta.UpdatedAt = time.Now()

	// Update in slice
	for i := range s.data.Workflows {
		if s.data.Workflows[i].ID == id {
			s.data.Workflows[i] = *meta
			break
		}
	}

	return s.save()
}

// IsEnabled checks if a workflow is enabled
func (s *workflowStore) IsEnabled(id string) (bool, error) {
	meta, err := s.GetMetaByID(id)
	if err != nil {
		return false, err
	}
	return meta.Enabled, nil
}
