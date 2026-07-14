package ports

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

type Checker struct {
	Configured func() []int
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

func (c Checker) Available(port int) error {
	for _, p := range c.Configured() {
		if p == port {
			return fmt.Errorf("port %d is configured by another instance", port)
		}
	}
	if c.Listening(port) {
		return fmt.Errorf("port %d is already listening", port)
	}
	return nil
}
