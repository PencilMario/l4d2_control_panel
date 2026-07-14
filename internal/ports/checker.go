package ports

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type Reservation struct {
	InstanceID string
	Port       int
}

type Checker struct {
	Configured func(context.Context) ([]Reservation, error)
	Listening  func(int) bool
}

func IsListening(port int) bool {
	raw := strconv.Itoa(port)
	connection, err := net.DialTimeout("tcp", "127.0.0.1:"+raw, 150*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		return true
	}
	udp, err := net.ListenPacket("udp", ":"+raw)
	if err != nil {
		return true
	}
	_ = udp.Close()
	return false
}

func Reservations(instances []domain.Instance) []Reservation {
	reservations := make([]Reservation, 0, len(instances)*2)
	for _, instance := range instances {
		ports := []int{instance.GamePort}
		if instance.SourceTVPort != 0 {
			ports = append(ports, instance.SourceTVPort)
		}
		ports = append(ports, instance.PluginPorts...)
		for _, port := range ports {
			reservations = append(reservations, Reservation{InstanceID: instance.ID, Port: port})
		}
	}
	return reservations
}

func (c Checker) Available(ctx context.Context, instanceID string, requested []int) error {
	seen := make(map[int]struct{}, len(requested))
	for _, port := range requested {
		if port < 1024 || port > 65535 {
			return fmt.Errorf("port %d is outside the allowed range", port)
		}
		if _, exists := seen[port]; exists {
			return fmt.Errorf("port %d is declared more than once", port)
		}
		seen[port] = struct{}{}
	}
	configured := []Reservation{}
	if c.Configured != nil {
		var err error
		configured, err = c.Configured(ctx)
		if err != nil {
			return err
		}
	}
	for _, port := range requested {
		for _, reservation := range configured {
			if reservation.InstanceID != instanceID && reservation.Port == port {
				return fmt.Errorf("port %d is configured by instance %s", port, reservation.InstanceID)
			}
		}
		if c.Listening != nil && c.Listening(port) {
			return fmt.Errorf("port %d is already listening", port)
		}
	}
	return nil
}
