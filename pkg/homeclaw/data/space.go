// Package data provides data access layer for HomeClaw.
package data

import (
	"strings"
)

// SpaceStore defines the interface for space data operations.
// Name is the primary key for all operations.
type SpaceStore interface {
	GetAll() ([]Space, error)
	Save(space Space) error
	Delete(name string) error
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

// Save saves a space (insert or update by Name)
func (s *spaceStore) Save(space Space) error {
	for i := range s.data.Spaces {
		if strings.EqualFold(s.data.Spaces[i].Name, space.Name) {
			s.data.Spaces[i] = space
			return s.save()
		}
	}
	s.data.Spaces = append(s.data.Spaces, space)
	return s.save()
}

// Delete removes a space by name (case-insensitive)
func (s *spaceStore) Delete(name string) error {
	lower := strings.ToLower(name)
	for i := range s.data.Spaces {
		if strings.ToLower(s.data.Spaces[i].Name) == lower {
			s.data.Spaces = append(s.data.Spaces[:i], s.data.Spaces[i+1:]...)
			return s.save()
		}
	}
	return ErrRecordNotFound
}
