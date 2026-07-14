package auth

import (
	"crypto/rand"
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
}

func NewService() *Service { return &Service{sessions: make(map[string]time.Time)} }
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
	s.sessions[token] = time.Now().Add(24 * time.Hour)
	return token, nil
}
func (s *Service) Valid(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expiry, ok := s.sessions[token]
	return ok && time.Now().Before(expiry)
}
func (s *Service) Logout(token string) { s.mu.Lock(); delete(s.sessions, token); s.mu.Unlock() }
func derive(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
}
