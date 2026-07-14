package auth

import (
	"path/filepath"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/store"
)

func TestBootstrapLoginAndSession(t *testing.T) {
	s := NewService()
	if err := s.Bootstrap("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := s.Bootstrap("another password"); err != ErrAlreadyBootstrapped {
		t.Fatalf("got %v", err)
	}
	if _, err := s.Login("wrong"); err != ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
	token, err := s.Login("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid(token) {
		t.Fatal("expected valid session")
	}
	s.Logout(token)
	if s.Valid(token) {
		t.Fatal("expected revoked session")
	}
}

func TestBootstrapRejectsWeakPassword(t *testing.T) {
	if err := NewService().Bootstrap("short"); err != ErrWeakPassword {
		t.Fatalf("got %v", err)
	}
}

func TestPersistentServiceSurvivesDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewPersistentService(db)
	if err != nil {
		t.Fatal(err)
	}
	if service.Configured() {
		t.Fatal("fresh database unexpectedly configured")
	}
	if err := service.Bootstrap("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if !service.Configured() {
		t.Fatal("bootstrap did not configure service")
	}
	token, err := service.Login("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err = NewPersistentService(db)
	if err != nil {
		t.Fatal(err)
	}
	if !service.Valid(token) {
		t.Fatal("session did not survive database reopen")
	}
	if _, err := service.Login("correct horse battery staple"); err != nil {
		t.Fatalf("password did not survive database reopen: %v", err)
	}
	service.Logout(token)
	if service.Valid(token) {
		t.Fatal("persistent session was not revoked")
	}
}
