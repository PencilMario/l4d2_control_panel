package auth

import "testing"

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
