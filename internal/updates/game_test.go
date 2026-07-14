package updates

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"strings"
	"testing"
)

type gameRepo struct{ instance domain.Instance }

func (r *gameRepo) Instance(context.Context, string) (domain.Instance, error) { return r.instance, nil }
func (r *gameRepo) UpdateInstance(_ context.Context, v domain.Instance) error {
	r.instance = v
	return nil
}

type gameUpdater struct{ events *[]string }

func (u gameUpdater) UpdateGame(context.Context, string, domain.Instance) error {
	*u.events = append(*u.events, "steamcmd")
	return nil
}

type privateApplier struct{ events *[]string }

func (p privateApplier) Apply(context.Context, string) error {
	*p.events = append(*p.events, "private")
	return nil
}

type orderedLife struct{ events *[]string }

func (l orderedLife) Stop(context.Context, string) error {
	*l.events = append(*l.events, "stop")
	return nil
}
func (l orderedLife) Start(context.Context, string) error {
	*l.events = append(*l.events, "start")
	return nil
}
func TestGameUpdateStopsValidatesReappliesAndStarts(t *testing.T) {
	events := []string{}
	repo := &gameRepo{instance: domain.Instance{ID: "abc", ActualState: domain.StateRunning}}
	coordinator := GameCoordinator{Root: "/data", Instances: repo, Lifecycle: orderedLife{&events}, Updater: gameUpdater{&events}, Private: privateApplier{&events}}
	if err := coordinator.Update(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "stop,steamcmd,private,start" {
		t.Fatalf("events=%s", got)
	}
}
