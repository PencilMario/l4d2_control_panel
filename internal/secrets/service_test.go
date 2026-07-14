package secrets

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncryptedSecretSurvivesReopenWithoutPlaintext(t *testing.T) {
	root := t.TempDir()
	key, err := LoadOrCreateKey(filepath.Join(root, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "panel.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(db, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Set(context.Background(), "github_token", "ghp_super_secret"); err != nil {
		t.Fatal(err)
	}
	var ciphertext []byte
	if err := db.DB().QueryRow(`SELECT ciphertext FROM secrets WHERE name='github_token'`).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ciphertext), "ghp_super_secret") {
		t.Fatal("secret stored in plaintext")
	}
	_ = db.Close()
	db, _ = store.Open(dbPath)
	defer db.Close()
	service, _ = New(db, key)
	value, found, err := service.Get(context.Background(), "github_token")
	if err != nil || !found || value != "ghp_super_secret" {
		t.Fatalf("value=%q found=%v err=%v", value, found, err)
	}
}
