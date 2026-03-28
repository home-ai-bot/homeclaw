// Package data provides data access layer for HomeClaw.
package data

// MemberStore defines the interface for member data operations
type MemberStore interface {
	GetAll() ([]Member, error)
	Save(members ...Member) error
	Delete(name string) error
}

// memberStore implements MemberStore using JSONStore
type memberStore struct {
	store *JSONStore
	data  MembersData
}

// NewMemberStore creates a new MemberStore
func NewMemberStore(store *JSONStore) (MemberStore, error) {
	s := &memberStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads data from file
func (s *memberStore) load() error {
	s.data = MembersData{Version: "1", Members: []Member{}}
	return s.store.Read("members", &s.data)
}

// save writes data to file
func (s *memberStore) save() error {
	return s.store.Write("members", s.data)
}

// GetAll returns all members
func (s *memberStore) GetAll() ([]Member, error) {
	return s.data.Members, nil
}

// Save saves members (insert or update)
func (s *memberStore) Save(members ...Member) error {
	for _, member := range members {
		found := false
		for i := range s.data.Members {
			if s.data.Members[i].Name == member.Name {
				s.data.Members[i] = member
				found = true
				break
			}
		}
		if !found {
			s.data.Members = append(s.data.Members, member)
		}
	}
	return s.save()
}

// Delete deletes a member by name
func (s *memberStore) Delete(name string) error {
	for i := range s.data.Members {
		if s.data.Members[i].Name == name {
			s.data.Members = append(s.data.Members[:i], s.data.Members[i+1:]...)
			return s.save()
		}
	}
	return ErrRecordNotFound
}
