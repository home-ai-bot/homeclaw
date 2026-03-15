// Package data provides data access layer for HomeClaw.
package data

// MemberStore defines the interface for member data operations
type MemberStore interface {
	GetAll() ([]Member, error)
	GetByName(name string) (*Member, error)
	GetByChannelID(channel string, channelUserID string) (*Member, error)
	Save(member Member) error
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

// GetByName finds a member by name
func (s *memberStore) GetByName(name string) (*Member, error) {
	for i := range s.data.Members {
		if s.data.Members[i].Name == name {
			return &s.data.Members[i], nil
		}
	}
	return nil, ErrRecordNotFound
}

// GetByChannelID finds a member by channel binding
func (s *memberStore) GetByChannelID(channel string, channelUserID string) (*Member, error) {
	for i := range s.data.Members {
		if info, ok := s.data.Members[i].Channels[channel]; ok {
			if info.UserID == channelUserID {
				return &s.data.Members[i], nil
			}
		}
	}
	return nil, ErrRecordNotFound
}

// Save saves a member (insert or update)
func (s *memberStore) Save(member Member) error {
	for i := range s.data.Members {
		if s.data.Members[i].Name == member.Name {
			s.data.Members[i] = member
			return s.save()
		}
	}
	s.data.Members = append(s.data.Members, member)
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
