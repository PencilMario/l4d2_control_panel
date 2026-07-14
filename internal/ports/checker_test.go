package ports

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

func TestCheckerRejectsConfiguredAndListeningPortsButExcludesCurrentInstance(t *testing.T) {
	c := Checker{
		Configured: func(context.Context) ([]Reservation, error) {
			return []Reservation{{InstanceID: "other", Port: 27015}, {InstanceID: "self", Port: 27017}}, nil
		},
		Listening: func(port int) bool { return port == 27016 },
	}
	if err := c.Available(context.Background(), "self", []int{27015}); err == nil {
		t.Fatal("configured collision accepted")
	}
	if err := c.Available(context.Background(), "self", []int{27016}); err == nil {
		t.Fatal("listener collision accepted")
	}
	if err := c.Available(context.Background(), "self", []int{27017}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckerRejectsDuplicateDeclarationsAndProviderErrors(t *testing.T) {
	c := Checker{Configured: func(context.Context) ([]Reservation, error) { return nil, nil }, Listening: func(int) bool { return false }}
	if err := c.Available(context.Background(), "self", []int{27015, 27015}); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate error=%v", err)
	}
	want := errors.New("database unavailable")
	c.Configured = func(context.Context) ([]Reservation, error) { return nil, want }
	if err := c.Available(context.Background(), "self", []int{27015}); !errors.Is(err, want) {
		t.Fatalf("provider error=%v", err)
	}
}

func TestReservationsIncludesEveryDeclaredPort(t *testing.T) {
	instances := []domain.Instance{{ID: "one", GamePort: 27015, SourceTVPort: 27020, PluginPorts: []int{27021, 27022}}, {ID: "two", GamePort: 27030}}
	want := []Reservation{{InstanceID: "one", Port: 27015}, {InstanceID: "one", Port: 27020}, {InstanceID: "one", Port: 27021}, {InstanceID: "one", Port: 27022}, {InstanceID: "two", Port: 27030}}
	if got := Reservations(instances); !reflect.DeepEqual(got, want) {
		t.Fatalf("reservations=%#v want=%#v", got, want)
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
