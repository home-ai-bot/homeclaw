// Package data provides data access layer for HomeClaw.
package data

import (
	"strings"
)

// SpaceStore defines the interface for space data operations
type SpaceStore interface {
	GetAll() ([]Space, error)
	GetByID(id string) (*Space, error)
	Save(space Space) error
	Delete(id string) error
	FindByName(name string) (*Space, error)
}

// spaceStore implements SpaceStore using JSONStore
type spaceStore struct {
	store *JSONStore
	data  SpacesData
}

// NewSpaceStore creates a new SpaceStore
func NewSpaceStore(store *JSONStore) (SpaceStore, error) {
	s := &spaceStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads data from file
func (s *spaceStore) load() error {
	s.data = SpacesData{Version: "1", Spaces: []Space{}}
	return s.store.Read("spaces", &s.data)
}

// save writes data to file
func (s *spaceStore) save() error {
	return s.store.Write("spaces", s.data)
}

// GetAll returns all spaces
func (s *spaceStore) GetAll() ([]Space, error) {
	return s.data.Spaces, nil
}

// GetByID finds a space by ID (recursively searches in children)
func (s *spaceStore) GetByID(id string) (*Space, error) {
	return s.findByIDRecursive(s.data.Spaces, id)
}

// findByIDRecursive recursively searches for a space by ID
func (s *spaceStore) findByIDRecursive(spaces []Space, id string) (*Space, error) {
	for i := range spaces {
		if spaces[i].ID == id {
			return &spaces[i], nil
		}
		if len(spaces[i].Children) > 0 {
			if found, err := s.findByIDRecursive(spaces[i].Children, id); err == nil {
				return found, nil
			}
		}
	}
	return nil, ErrRecordNotFound
}

// Save saves a space (insert or update)
func (s *spaceStore) Save(space Space) error {
	// Try to find and update existing space
	if updated := s.updateRecursive(&s.data.Spaces, space); updated {
		return s.save()
	}

	// Insert new space
	s.data.Spaces = append(s.data.Spaces, space)
	return s.save()
}

// updateRecursive recursively updates a space in the tree
func (s *spaceStore) updateRecursive(spaces *[]Space, space Space) bool {
	for i := range *spaces {
		if (*spaces)[i].ID == space.ID {
			(*spaces)[i] = space
			return true
		}
		if len((*spaces)[i].Children) > 0 {
			if s.updateRecursive(&(*spaces)[i].Children, space) {
				return true
			}
		}
	}
	return false
}

// Delete deletes a space by ID
func (s *spaceStore) Delete(id string) error {
	if deleted := s.deleteRecursive(&s.data.Spaces, id); deleted {
		return s.save()
	}
	return ErrRecordNotFound
}

// deleteRecursive recursively deletes a space from the tree
func (s *spaceStore) deleteRecursive(spaces *[]Space, id string) bool {
	for i := range *spaces {
		if (*spaces)[i].ID == id {
			// Found it, remove from slice
			*spaces = append((*spaces)[:i], (*spaces)[i+1:]...)
			return true
		}
		if len((*spaces)[i].Children) > 0 {
			if s.deleteRecursive(&(*spaces)[i].Children, id) {
				return true
			}
		}
	}
	return false
}

// FindByName finds a space by name (case-insensitive, recursive)
func (s *spaceStore) FindByName(name string) (*Space, error) {
	return s.findByNameRecursive(s.data.Spaces, name)
}

// findByNameRecursive recursively searches for a space by name
func (s *spaceStore) findByNameRecursive(spaces []Space, name string) (*Space, error) {
	lowerName := strings.ToLower(name)
	for i := range spaces {
		if strings.ToLower(spaces[i].Name) == lowerName {
			return &spaces[i], nil
		}
		if len(spaces[i].Children) > 0 {
			if found, err := s.findByNameRecursive(spaces[i].Children, name); err == nil {
				return found, nil
			}
		}
	}
	return nil, ErrRecordNotFound
}
