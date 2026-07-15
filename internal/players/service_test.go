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

type summaryQuery struct{ playerCalls int }

func (*summaryQuery) Info(string) (a2s.Info, error) {
	return a2s.Info{Map: "c5m1_waterfront", Players: 3, MaxPlayers: 12}, nil
}
func (q *summaryQuery) Players(string) ([]a2s.Player, error) {
	q.playerCalls++
	return nil, nil
}

type summaryConsole struct{ statusCalls int }

func (c *summaryConsole) Status(context.Context, string) (string, error) {
	c.statusCalls++
	return "", nil
}
func (*summaryConsole) PlayerCommand(context.Context, string, string) error { return nil }

func TestSummaryUsesA2SInfoWithoutDetailedPlayerOrConsoleQueries(t *testing.T) {
	query := &summaryQuery{}
	console := &summaryConsole{}
	service := NewService(fakeInstances{domain.Instance{ID: "abc", ContainerID: "container", GamePort: 27015}}, query, console, "127.0.0.1")

	summary, err := service.Summary(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Map != "c5m1_waterfront" || summary.Players != 3 || summary.MaxPlayers != 12 {
		t.Fatalf("summary=%#v", summary)
	}
	if query.playerCalls != 0 || console.statusCalls != 0 {
		t.Fatalf("playerCalls=%d statusCalls=%d", query.playerCalls, console.statusCalls)
	}
}

func TestServiceJoinsA2SWithStatusUserIDAndExecutesBan(t *testing.T) {
	console := &fakeConsole{}
	service := NewService(fakeInstances{domain.Instance{ID: "abc", ContainerID: "container", GamePort: 27015}}, fakeQuery{}, console, "127.0.0.1")
	snapshot, err := service.Online(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Map != "map" || len(snapshot.Players) != 1 || snapshot.Players[0].UserID != 12 || snapshot.Players[0].Score == nil || *snapshot.Players[0].Score != 7 || snapshot.Players[0].Duration != 90*time.Second {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if err := service.Ban(context.Background(), "abc", 12, 30); err != nil {
		t.Fatal(err)
	}
	if console.command != "banid 30 12 kick; writeid" {
		t.Fatalf("command=%q", console.command)
	}
}

type operationsQuery struct{}

func (operationsQuery) Info(string) (a2s.Info, error) {
	return a2s.Info{Map: "fallback-map", Players: 1, MaxPlayers: 8}, nil
}
func (operationsQuery) Players(string) ([]a2s.Player, error) {
	return []a2s.Player{{Name: "Sir.P", Score: 12, Duration: 48}}, nil
}

type operationsConsole struct{}

func (operationsConsole) Status(context.Context, string) (string, error) {
	return `hostname: 6
version : 2.2.4.3 10097 secure  (unknown)
udp/ip  : 127.0.1.1:27991 [ public 221.215.78.153:27991 ]
os      : Linux Dedicated
map     : c2m1_highway
players : 1 humans, 4 bots (12 max)
# userid name uniqueid connected ping loss state rate
#  2 1 "Sir.P" STEAM_1:0:526095818 00:48 29 0 active 100000
# 3 "Rochelle" BOT active`, nil
}
func (operationsConsole) PlayerCommand(context.Context, string, string) error { return nil }

func TestServiceReturnsMatchAndHumanOperationsWithA2SScore(t *testing.T) {
	service := NewService(fakeInstances{domain.Instance{ID: "abc", ContainerID: "container", GamePort: 27015}}, operationsQuery{}, operationsConsole{}, "127.0.0.1")
	snapshot, err := service.Online(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Match.Map != "c2m1_highway" || snapshot.Match.Hostname != "6" || snapshot.Map != "c2m1_highway" || snapshot.MaxPlayers != 12 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if len(snapshot.Players) != 1 {
		t.Fatalf("players=%#v", snapshot.Players)
	}
	player := snapshot.Players[0]
	if player.UserID != 2 || player.UniqueID != "STEAM_1:0:526095818" || player.Connected != "00:48" || player.Ping != 29 || player.Loss != 0 || player.Score == nil || *player.Score != 12 || player.Duration != 48*time.Second {
		t.Fatalf("player=%#v", player)
	}
}
