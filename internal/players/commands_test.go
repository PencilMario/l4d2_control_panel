package players

import "testing"

func TestCommandsUseNumericUserID(t *testing.T) {
	cmd, err := Kick(42)
	if err != nil || cmd != "kickid 42" {
		t.Fatalf("%q %v", cmd, err)
	}
	if _, err := Kick(0); err == nil {
		t.Fatal("accepted invalid id")
	}
	cmd, err = Ban(42, 30)
	if err != nil || cmd != "banid 30 42 kick; writeid" {
		t.Fatalf("%q %v", cmd, err)
	}
	cmd, _ = Ban(42, 0)
	if cmd != "banid 0 42 kick; writeid" {
		t.Fatalf("%q", cmd)
	}
}
