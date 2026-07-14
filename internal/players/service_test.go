package players

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/a2s"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"testing"
	"time"
)

type fakeInstances struct{ instance domain.Instance }

func (f fakeInstances) Instance(context.Context, string) (domain.Instance, error) {
	return f.instance, nil
}

type fakeQuery struct{}

func (fakeQuery) Info(string) (a2s.Info, error) {
	return a2s.Info{Map: "map", Players: 1, MaxPlayers: 8}, nil
}
func (fakeQuery) Players(string) ([]a2s.Player, error) {
	return []a2s.Player{{Name: "Coach", Score: 7, Duration: 90}}, nil
}

type fakeConsole struct{ command string }

func (f *fakeConsole) Status(context.Context, string) (string, error) {
	return `# 12 "Coach" STEAM_1:1:42 01:30 45 0 active 30000 1.2.3.4:27005`, nil
}
func (f *fakeConsole) PlayerCommand(_ context.Context, _ string, command string) error {
	f.command = command
	return nil
}
func TestServiceJoinsA2SWithStatusUserIDAndExecutesBan(t *testing.T) {
	console := &fakeConsole{}
	service := NewService(fakeInstances{domain.Instance{ID: "abc", ContainerID: "container", GamePort: 27015}}, fakeQuery{}, console, "127.0.0.1")
	snapshot, err := service.Online(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Map != "map" || len(snapshot.Players) != 1 || snapshot.Players[0].UserID != 12 || snapshot.Players[0].Score != 7 || snapshot.Players[0].Duration != 90*time.Second {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if err := service.Ban(context.Background(), "abc", 12, 30); err != nil {
		t.Fatal(err)
	}
	if console.command != "banid 30 12 kick; writeid" {
		t.Fatalf("command=%q", console.command)
	}
}
