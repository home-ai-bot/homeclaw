// Package data provides data access layer for HomeClaw.
package data

import "time"

// XiaomiAccount 表示小米账号及其 OAuth2 凭据
type XiaomiAccount struct {
	ID             string    `json:"id"`               // 小米账号唯一标识（userId）
	ClientID       string    `json:"client_id"`        // OAuth2 客户端 ID
	AccessToken    string    `json:"access_token"`     // 访问令牌
	RefreshToken   string    `json:"refresh_token"`    // 刷新令牌
	ExpiresIn      int       `json:"expires_in"`       // 令牌有效期（秒）
	TokenExpiresAt time.Time `json:"token_expires_at"` // 令牌绝对过期时间
	HomeID         string    `json:"home_id"`          // 关联的米家家庭 ID
	HomeName       string    `json:"home_name"`        // 关联的米家家庭名称
	CreatedAt      time.Time `json:"created_at"`       // 首次绑定时间
	UpdatedAt      time.Time `json:"updated_at"`       // 最近更新时间
}

// XiaomiAccountData 是 xiaomi-account.json 的根结构
type XiaomiAccountData struct {
	Version string        `json:"version"`
	Account XiaomiAccount `json:"account"`
}

// XiaomiAccountStore defines the interface for XiaomiAccount data operations
type XiaomiAccountStore interface {
	Get() (*XiaomiAccount, error)
	Save(account XiaomiAccount) error
	Delete() error
}

// xiaomiAccountStore implements XiaomiAccountStore using JSONStore
type xiaomiAccountStore struct {
	store *JSONStore
	data  XiaomiAccountData
}

// NewXiaomiAccountStore creates a new XiaomiAccountStore
func NewXiaomiAccountStore(store *JSONStore) (XiaomiAccountStore, error) {
	s := &xiaomiAccountStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads data from file
func (s *xiaomiAccountStore) load() error {
	s.data = XiaomiAccountData{Version: "1"}
	return s.store.Read("xiaomi-account", &s.data)
}

// save writes data to file
func (s *xiaomiAccountStore) save() error {
	return s.store.Write("xiaomi-account", s.data)
}

// Get returns the xiaomi account, or ErrRecordNotFound if not set
func (s *xiaomiAccountStore) Get() (*XiaomiAccount, error) {
	if s.data.Account.ID == "" {
		return nil, ErrRecordNotFound
	}
	acc := s.data.Account
	return &acc, nil
}

// Save saves the xiaomi account
func (s *xiaomiAccountStore) Save(account XiaomiAccount) error {
	now := time.Now()
	if s.data.Account.ID == account.ID && !s.data.Account.CreatedAt.IsZero() {
		account.CreatedAt = s.data.Account.CreatedAt
	} else if account.CreatedAt.IsZero() {
		account.CreatedAt = now
	}
	account.UpdatedAt = now
	s.data.Account = account
	return s.save()
}

// Delete clears the stored xiaomi account
func (s *xiaomiAccountStore) Delete() error {
	if s.data.Account.ID == "" {
		return ErrRecordNotFound
	}
	s.data.Account = XiaomiAccount{}
	return s.save()
}
