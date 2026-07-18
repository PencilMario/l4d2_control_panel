package overlayfs

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type recordingMounter struct {
	ensure Mount
}

func (m *recordingMounter) Preflight(context.Context) error { return nil }
func (m *recordingMounter) Ensure(_ context.Context, mount Mount) error {
	m.ensure = mount
	return nil
}
func (m *recordingMounter) Inspect(context.Context, Mount) (MountStatus, error) {
	return MountStatus{Mounted: true, Lower: m.ensure.Lower}, nil
}
func (m *recordingMounter) ResetManagedPaths(context.Context, Mount, []string) error { return nil }
func (m *recordingMounter) ResetUpper(context.Context, Mount) error                  { return nil }
func (m *recordingMounter) Unmount(context.Context, Mount) error                     { return nil }

func TestServerEnsuresCanonicalMount(t *testing.T) {
	mounter := &recordingMounter{}
	handler := NewServer(Paths{Root: t.TempDir()}, mounter)
	body, _ := json.Marshal(Request{InstanceID: "instance-1", ReleaseID: "release-1"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/ensure", bytes.NewReader(body)))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	want, _ := (Paths{Root: handler.paths.Root}).Mount("instance-1", "release-1")
	if !reflect.DeepEqual(mounter.ensure, want) {
		t.Fatalf("ensure = %#v, want %#v", mounter.ensure, want)
	}
}

func TestServerRejectsUnknownOperationAndUnsafePath(t *testing.T) {
	handler := NewServer(Paths{Root: t.TempDir()}, &recordingMounter{})
	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, httptest.NewRequest(http.MethodPost, "/v1/mount-anything", bytes.NewReader([]byte(`{}`))))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d", unknown.Code)
	}
	unsafe := httptest.NewRecorder()
	handler.ServeHTTP(unsafe, httptest.NewRequest(http.MethodPost, "/v1/ensure", bytes.NewReader([]byte(`{"instance_id":"../escape","release_id":"release"}`))))
	if unsafe.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe status = %d body=%s", unsafe.Code, unsafe.Body.String())
	}
}
