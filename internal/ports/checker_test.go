package ports

import (
	"net"
	"strconv"
	"testing"
)

func TestCheckerRejectsConfiguredAndListeningPorts(t *testing.T) {
	c := Checker{Configured: func() []int { return []int{27015} }, Listening: func(port int) bool { return port == 27016 }}
	if err := c.Available(27015); err == nil {
		t.Fatal("configured collision accepted")
	}
	if err := c.Available(27016); err == nil {
		t.Fatal("listener collision accepted")
	}
	if err := c.Available(27017); err != nil {
		t.Fatal(err)
	}
}

func TestIsListeningDetectsHostTCPPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, raw, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(raw)
	if !IsListening(port) {
		t.Fatalf("port %d not detected", port)
	}
}
