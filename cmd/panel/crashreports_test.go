package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type crashReportInstances struct {
	items []domain.Instance
	err   error
}

func (r crashReportInstances) Instances(context.Context) ([]domain.Instance, error) {
	return r.items, r.err
}

func TestCrashReportInstanceAuthorizerMatchesManagedServerID(t *testing.T) {
	root := t.TempDir()
	serverIDPath := filepath.Join(root, "instances", "instance-a", "game", "left4dead2", "addons", "sourcemod", "data", "dumps", "server-id.txt")
	if err := os.MkdirAll(filepath.Dir(serverIDPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serverIDPath, []byte("server-uuid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authorize := newCrashReportInstanceAuthorizer(root, crashReportInstances{items: []domain.Instance{{ID: "instance-a"}}})
	if err := authorize(context.Background(), "server-uuid", "left4dead2"); err != nil {
		t.Fatalf("known instance rejected: %v", err)
	}
	if err := authorize(context.Background(), "other", "left4dead2"); !errors.Is(err, crashreports.ErrInstanceNotAllowed) {
		t.Fatalf("unknown server error=%v", err)
	}
	if err := authorize(context.Background(), "server-uuid", "other-game"); !errors.Is(err, crashreports.ErrInstanceNotAllowed) {
		t.Fatalf("wrong game directory error=%v", err)
	}
}

func TestCrashReportInstanceResolverReturnsManagedInstanceID(t *testing.T) {
	root := t.TempDir()
	serverIDPath := filepath.Join(root, "instances", "instance-a", "game", "left4dead2", "addons", "sourcemod", "data", "dumps", "server-id.txt")
	if err := os.MkdirAll(filepath.Dir(serverIDPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serverIDPath, []byte("server-uuid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolve := newCrashReportInstanceResolver(root, crashReportInstances{items: []domain.Instance{{ID: "instance-a"}}})
	instanceID, err := resolve(context.Background(), "server-uuid", "left4dead2")
	if err != nil || instanceID != "instance-a" {
		t.Fatalf("resolved id=%q err=%v", instanceID, err)
	}
}

func TestCrashReportInstanceAuthorizerMatchesManagedOverlayGameLink(t *testing.T) {
	root := t.TempDir()
	mergedServerIDPath := filepath.Join(root, "instances", "instance-a", "overlay", "merged", "left4dead2", "addons", "sourcemod", "data", "dumps", "server-id.txt")
	if err := os.MkdirAll(filepath.Dir(mergedServerIDPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mergedServerIDPath, []byte("server-uuid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	instanceRoot := filepath.Join(root, "instances", "instance-a")
	if err := os.Symlink(filepath.Join("overlay", "merged"), filepath.Join(instanceRoot, "game")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	authorize := newCrashReportInstanceAuthorizer(root, crashReportInstances{items: []domain.Instance{{ID: "instance-a"}}})
	if err := authorize(context.Background(), "server-uuid", "left4dead2"); err != nil {
		t.Fatalf("managed overlay game link rejected: %v", err)
	}
}

func TestCrashReportInstanceAuthorizerFailsClosedAndPropagatesRepositoryError(t *testing.T) {
	root := t.TempDir()
	authorize := newCrashReportInstanceAuthorizer(root, crashReportInstances{items: []domain.Instance{{ID: "missing"}}})
	if err := authorize(context.Background(), "server-uuid", "left4dead2"); !errors.Is(err, crashreports.ErrInstanceNotAllowed) {
		t.Fatalf("missing server id error=%v", err)
	}
	want := errors.New("database unavailable")
	authorize = newCrashReportInstanceAuthorizer(root, crashReportInstances{err: want})
	if err := authorize(context.Background(), "server-uuid", "left4dead2"); !errors.Is(err, want) {
		t.Fatalf("repository error=%v want=%v", err, want)
	}
}

func TestCrashReportInstanceAuthorizerRejectsSymlinkedInstanceParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	serverIDPath := filepath.Join(outside, "game", "left4dead2", "addons", "sourcemod", "data", "dumps", "server-id.txt")
	if err := os.MkdirAll(filepath.Dir(serverIDPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serverIDPath, []byte("server-uuid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	instancesRoot := filepath.Join(root, "instances")
	if err := os.MkdirAll(instancesRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(instancesRoot, "instance-a")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	authorize := newCrashReportInstanceAuthorizer(root, crashReportInstances{items: []domain.Instance{{ID: "instance-a"}}})
	if err := authorize(context.Background(), "server-uuid", "left4dead2"); !errors.Is(err, crashreports.ErrInstanceNotAllowed) {
		t.Fatalf("symlinked instance parent was accepted: %v", err)
	}
}
