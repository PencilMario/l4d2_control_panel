package scheduler

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"path/filepath"
	"testing"
)

type fakeDispatcher struct{ ran string }

func (f *fakeDispatcher) Dispatch(_ context.Context, task domain.ScheduledTask) error {
	f.ran = task.ID
	return nil
}
func TestServicePersistsSchedulesAndRunsSharedDispatcher(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dispatcher := &fakeDispatcher{}
	service := NewService(db, dispatcher)
	task := domain.ScheduledTask{ID: "task-1", InstanceID: "abc", Type: "game_update", Cron: "0 4 * * *", Timezone: "Asia/Hong_Kong", OnlinePolicy: "skip", Payload: `{}`, Enabled: true}
	if err := service.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	tasks, _ := db.ScheduledTasks(context.Background())
	if len(tasks) != 1 || tasks[0].NextRun.IsZero() {
		t.Fatalf("tasks=%#v", tasks)
	}
	if err := service.RunNow(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if dispatcher.ran != "task-1" {
		t.Fatalf("ran=%q", dispatcher.ran)
	}
}
