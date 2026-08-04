package a2sdefense

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestClientAppliesConfigAndReadsStatusOverUnixSocket(t *testing.T) {
	socket := shortTestSocket(t)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	firewall := &fakeFirewall{status: Status{Compatible: true, Enabled: true, Revision: 8, PolicyVersion: PolicyVersion, Ports: []int{27015}}}
	server := &http.Server{Handler: NewServer(firewall)}
	go server.Serve(listener)
	defer server.Close()

	client := NewClient(socket)
	status, err := client.Apply(context.Background(), Config{Version: APIVersion, Enabled: true, Ports: []int{27015}, Revision: 8})
	if err != nil {
		t.Fatal(err)
	}
	if status.Revision != 8 || firewall.config.Revision != 8 {
		t.Fatalf("status=%+v config=%+v", status, firewall.config)
	}
	status, err = client.Status(context.Background())
	if err != nil || !status.Enabled {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestClientReturnsHelperError(t *testing.T) {
	socket := shortTestSocket(t)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: NewServer(&fakeFirewall{err: ErrStaleRevision})}
	go server.Serve(listener)
	defer server.Close()
	if _, err := NewClient(socket).Apply(context.Background(), Config{Version: APIVersion, Revision: 1}); err == nil {
		t.Fatal("expected helper error")
	}
}

func shortTestSocket(t *testing.T) string {
	t.Helper()
	path := filepath.Join(os.TempDir(), fmt.Sprintf("a2s-%d.sock", os.Getpid()))
	_ = os.Remove(path)
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}
