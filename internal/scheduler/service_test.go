package scheduler

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"path/filepath"
	"testing"
	"time"
)

type fakeDispatcher struct{ ran string }

func (f *fakeDispatcher) Dispatch(_ context.Context, task domain.ScheduledTask) error {
	f.ran = task.ID
	return nil
}

func TestServiceUpdatesDisablesAndDeletesExistingSchedule(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(db, &fakeDispatcher{})
	defer service.Stop()
	ctx := context.Background()
	lastRun := time.Date(2026, time.July, 15, 20, 0, 0, 0, time.UTC)
	task := domain.ScheduledTask{ID: "task-1", Type: "game_update", Cron: "0 4 * * *", Timezone: "Asia/Hong_Kong", OnlinePolicy: "skip", Payload: `{}`, Enabled: true, LastRun: lastRun}
	if err := service.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	_, scheduled := service.entries[task.ID]
	service.mu.Unlock()
	if !scheduled {
		t.Fatal("enabled task was not registered with cron")
	}

	task.Cron = "30 5 * * *"
	task.OnlinePolicy = "wait"
	task.Enabled = false
	if err := service.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	_, scheduled = service.entries[task.ID]
	service.mu.Unlock()
	if scheduled {
		t.Fatal("disabled task remained registered with cron")
	}
	tasks, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != task.ID || tasks[0].Cron != "30 5 * * *" || tasks[0].OnlinePolicy != "wait" || tasks[0].Enabled || !tasks[0].LastRun.Equal(lastRun) || tasks[0].NextRun.IsZero() {
		t.Fatalf("updated tasks=%#v", tasks)
	}

	if err := service.Delete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	tasks, err = service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks after delete=%#v", tasks)
	}
}
func TestServicePersistsSchedulesAndRunsSharedDispatcher(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dispatcher := &fakeDispatcher{}
	service := NewService(db, dispatcher)
	task := domain.ScheduledTask{ID: "task-1", Type: "game_update", Cron: "0 4 * * *", Timezone: "Asia/Hong_Kong", OnlinePolicy: "skip", Payload: `{}`, Enabled: true}
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

func TestServiceValidatesTaskScope(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(db, &fakeDispatcher{})
	defer service.Stop()
	base := domain.ScheduledTask{ID: "task", Cron: "0 4 * * *", Timezone: "UTC", OnlinePolicy: "force", Payload: `{}`}
	global := base
	global.Type = "game_update"
	global.InstanceID = "abc"
	if err := service.Save(context.Background(), global); err == nil {
		t.Fatal("global game update accepted instance")
	}
	instanceTask := base
	instanceTask.Type = "backup"
	if err := service.Save(context.Background(), instanceTask); err == nil {
		t.Fatal("instance task accepted empty instance")
	}
}
