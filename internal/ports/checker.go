package ports

import "fmt"

type Checker struct {
	Configured func() []int
	Listening  func(int) bool
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
