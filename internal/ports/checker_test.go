package ports

import "testing"

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
