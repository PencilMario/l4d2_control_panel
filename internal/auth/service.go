package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrAlreadyBootstrapped = errors.New("administrator already configured")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrWeakPassword        = errors.New("password must contain at least 12 characters")
)

type Service struct {
	mu       sync.RWMutex
	password []byte
	salt     []byte
	sessions map[string]time.Time
	repo     Repository
}

type Repository interface {
	LoadCredential() (hash, salt []byte, found bool, err error)
	SaveCredential(hash, salt []byte) error
	SaveSession(tokenHash []byte, expires time.Time) error
	SessionValid(tokenHash []byte, now time.Time) (bool, error)
	DeleteSession(tokenHash []byte) error
}

func NewService() *Service { return &Service{sessions: make(map[string]time.Time)} }
func NewPersistentService(repo Repository) (*Service, error) {
	hash, salt, _, err := repo.LoadCredential()
	if err != nil {
		return nil, err
	}
	return &Service{password: hash, salt: salt, sessions: make(map[string]time.Time), repo: repo}, nil
}
func (s *Service) Configured() bool { s.mu.RLock(); defer s.mu.RUnlock(); return len(s.password) > 0 }
func (s *Service) Bootstrap(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.password) > 0 {
		return ErrAlreadyBootstrapped
	}
	if len(password) < 12 {
		return ErrWeakPassword
	}
	s.salt = make([]byte, 16)
	if _, err := rand.Read(s.salt); err != nil {
		return err
	}
	s.password = derive(password, s.salt)
	if s.repo != nil {
		if err := s.repo.SaveCredential(s.password, s.salt); err != nil {
			s.password = nil
			s.salt = nil
			return err
		}
	}
	return nil
}
func (s *Service) Login(password string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.password) == 0 || subtle.ConstantTimeCompare(s.password, derive(password, s.salt)) != 1 {
		return "", ErrInvalidCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(24 * time.Hour)
	if s.repo != nil {
		hash := sha256.Sum256([]byte(token))
		if err := s.repo.SaveSession(hash[:], expires); err != nil {
			return "", err
		}
	} else {
		s.sessions[token] = expires
	}
	return token, nil
}
func (s *Service) Valid(token string) bool {
	if s.repo != nil {
		hash := sha256.Sum256([]byte(token))
		valid, err := s.repo.SessionValid(hash[:], time.Now())
		return err == nil && valid
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	expiry, ok := s.sessions[token]
	return ok && time.Now().Before(expiry)
}
func (s *Service) Logout(token string) {
	if s.repo != nil {
		hash := sha256.Sum256([]byte(token))
		_ = s.repo.DeleteSession(hash[:])
		return
	}
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}
func derive(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
}
