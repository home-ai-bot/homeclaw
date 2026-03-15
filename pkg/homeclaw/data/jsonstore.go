// Package data provides data access layer for HomeClaw.
// All JSON files use backup mechanism to prevent data loss on power failure.
package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sipeed/picoclaw/pkg/fileutil"
)

var (
	// ErrFileCorrupted is returned when both main file and backup are corrupted
	ErrFileCorrupted = errors.New("data file corrupted, please restore from backup manually")
	// ErrRecordNotFound is returned when a record is not found
	ErrRecordNotFound = errors.New("record not found")
)

// JSONStore provides atomic file operations with backup mechanism.
// File saving process:
//  1. Copy current file to .bak (if exists)
//  2. Write new data to file using atomic write (temp + rename)
//
// File reading process:
//  1. Try to read main file
//  2. If main file is corrupted, try to read .bak
//  3. If both are corrupted, return ErrFileCorrupted
//
// This ensures data safety even if power is lost during write.
type JSONStore struct {
	dir   string
	mutex sync.Mutex
}

// NewJSONStore creates a new JSON store at the specified directory.
func NewJSONStore(dir string) (*JSONStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("data: create directory: %w", err)
	}
	return &JSONStore{dir: dir}, nil
}

// filePath returns the full path for a data file.
func (s *JSONStore) filePath(name string) string {
	return filepath.Join(s.dir, name+".json")
}

// backupPath returns the full path for a backup file.
func (s *JSONStore) backupPath(name string) string {
	return filepath.Join(s.dir, name+".json.bak")
}

// Read reads JSON data from file. If main file is corrupted, tries backup.
// Returns ErrFileCorrupted if both files are corrupted.
func (s *JSONStore) Read(name string, v interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	mainPath := s.filePath(name)
	backupPath := s.backupPath(name)

	// Try main file first
	data, err := os.ReadFile(mainPath)
	if err == nil {
		if decodeErr := json.Unmarshal(data, v); decodeErr == nil {
			return nil
		}
		// Main file exists but corrupted, will try backup
	}

	// If main file doesn't exist, that's ok for new files
	if os.IsNotExist(err) {
		// Return zero value for new files
		return nil
	}

	// Try backup file
	backupData, backupErr := os.ReadFile(backupPath)
	if backupErr == nil {
		if decodeErr := json.Unmarshal(backupData, v); decodeErr == nil {
			// Backup is valid, try to restore main file
			_ = fileutil.WriteFileAtomic(mainPath, backupData, 0o644)
			return nil
		}
		// Backup also corrupted
	}

	// Both files are corrupted or unreadable
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrFileCorrupted, name)
	}

	// Main file exists but empty/corrupted, backup doesn't exist
	return fmt.Errorf("%w: %s", ErrFileCorrupted, name)
}

// Write writes JSON data to file with backup mechanism.
// Process: copy current to .bak, then atomic write new data.
func (s *JSONStore) Write(name string, v interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	mainPath := s.filePath(name)
	backupPath := s.backupPath(name)

	// If main file exists, copy it to backup first
	if data, err := os.ReadFile(mainPath); err == nil {
		if writeErr := fileutil.WriteFileAtomic(backupPath, data, 0o644); writeErr != nil {
			// Log but continue - better to proceed than fail completely
			// The atomic write below will still protect data
		}
	}

	// Marshal data
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("data: marshal %s: %w", name, err)
	}

	// Atomic write to main file
	if err := fileutil.WriteFileAtomic(mainPath, data, 0o644); err != nil {
		return fmt.Errorf("data: write %s: %w", name, err)
	}

	return nil
}

// Exists checks if a data file exists.
func (s *JSONStore) Exists(name string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, err := os.Stat(s.filePath(name))
	return err == nil
}

// Remove deletes a data file and its backup.
func (s *JSONStore) Remove(name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	mainPath := s.filePath(name)
	backupPath := s.backupPath(name)

	var errs []error
	if err := os.Remove(mainPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("data: remove %s: %v", name, errs)
	}
	return nil
}

// List returns all data file names (without extension) in the store.
func (s *JSONStore) List() ([]string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("data: list directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip backup files and non-json files
		if filepath.Ext(name) == ".json" && !strings.HasSuffix(name, ".bak") {
			names = append(names, name[:len(name)-5]) // Remove .json
		}
	}
	return names, nil
}
