package content

import (
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/store"
)

func TestSelfServiceVPKSchedulerRunAndLifecycle(t *testing.T) {
	uploads, _ := NewUploadManager(t.TempDir())
	repo := &fakeSelfServiceVPKStore{settings: store.SelfServiceVPKSettings{AutoDelete: true}, items: map[string]store.SelfServiceVPK{}}
	scheduler := NewSelfServiceVPKScheduler(NewSelfServiceVPKManager(repo, uploads))
	if err := scheduler.Start(); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(); err != nil {
		t.Fatalf("second start: %v", err)
	}
	result, err := scheduler.RunNow(time.Now().UTC())
	if err != nil || result.Scanned != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	scheduler.Stop()
	scheduler.Stop()
}
