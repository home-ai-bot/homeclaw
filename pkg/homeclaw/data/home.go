// Package data provides data access layer for HomeClaw.
package data

// HomeStore defines the interface for home data operations
type HomeStore interface {
	GetAll() ([]Home, error)
	Save(home Home) error
	Delete(id string) error
}

// homeStore implements HomeStore using JSONStore
type homeStore struct {
	store *JSONStore
	data  HomesData
}

// NewHomeStore creates a new HomeStore
func NewHomeStore(store *JSONStore) (HomeStore, error) {
	s := &homeStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads data from file
func (s *homeStore) load() error {
	s.data = HomesData{Version: "1", Homes: []Home{}}
	return s.store.Read("homes", &s.data)
}

// save writes data to file
func (s *homeStore) save() error {
	return s.store.Write("homes", s.data)
}

// GetAll returns all homes
func (s *homeStore) GetAll() ([]Home, error) {
	return s.data.Homes, nil
}

// Save saves a home (insert or update)
func (s *homeStore) Save(home Home) error {
	for i := range s.data.Homes {
		if s.data.Homes[i].FromID == home.FromID {
			s.data.Homes[i] = home
			return s.save()
		}
	}
	s.data.Homes = append(s.data.Homes, home)
	return s.save()
}

// Delete deletes a home by FromID
func (s *homeStore) Delete(id string) error {
	for i := range s.data.Homes {
		if s.data.Homes[i].FromID == id {
			s.data.Homes = append(s.data.Homes[:i], s.data.Homes[i+1:]...)
			return s.save()
		}
	}
	return ErrRecordNotFound
}
