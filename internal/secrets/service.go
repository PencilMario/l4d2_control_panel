package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type Repository interface {
	SaveSecret(context.Context, string, []byte) error
	LoadSecret(context.Context, string) ([]byte, bool, error)
	DeleteSecret(context.Context, string) error
}
type Service struct {
	repo Repository
	aead cipher.AEAD
}

func LoadOrCreateKey(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		if len(raw) != 32 {
			return nil, errors.New("secret key must be 32 bytes")
		}
		return raw, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	raw = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return raw, nil
}
func New(repo Repository, key []byte) (*Service, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, aead: aead}, nil
}
func (s *Service) Set(ctx context.Context, name, value string) error {
	allowed := map[string]bool{"github_token": true, "steam_username": true, "steam_password": true}
	if !allowed[name] || value == "" {
		return errors.New("unsupported or empty secret")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(value), []byte(name))
	return s.repo.SaveSecret(ctx, name, sealed)
}
func (s *Service) Get(ctx context.Context, name string) (string, bool, error) {
	sealed, found, err := s.repo.LoadSecret(ctx, name)
	if err != nil || !found {
		return "", found, err
	}
	nonceSize := s.aead.NonceSize()
	if len(sealed) < nonceSize {
		return "", false, errors.New("invalid encrypted secret")
	}
	plain, err := s.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], []byte(name))
	if err != nil {
		return "", false, err
	}
	return string(plain), true, nil
}
func (s *Service) Delete(ctx context.Context, name string) error {
	return s.repo.DeleteSecret(ctx, name)
}
