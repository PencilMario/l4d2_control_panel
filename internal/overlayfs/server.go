package overlayfs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
)

type MountStatus struct {
	Mounted bool   `json:"mounted"`
	Lower   string `json:"lower,omitempty"`
}

type Mounter interface {
	Preflight(context.Context) error
	Ensure(context.Context, Mount) error
	Inspect(context.Context, Mount) (MountStatus, error)
	ResetManagedPaths(context.Context, Mount, []string) error
	ResetUpper(context.Context, Mount) error
	Unmount(context.Context, Mount) error
}

type Request struct {
	InstanceID string   `json:"instance_id"`
	ReleaseID  string   `json:"release_id"`
	Paths      []string `json:"paths,omitempty"`
}

type Server struct {
	paths   Paths
	mounter Mounter
}

func NewServer(paths Paths, mounter Mounter) *Server {
	return &Server{paths: paths, mounter: mounter}
}

func (s *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.URL.Path == "/v1/preflight" {
		if err := s.mounter.Preflight(request.Context()); err != nil {
			writeServerError(response, http.StatusServiceUnavailable, err)
			return
		}
		response.WriteHeader(http.StatusNoContent)
		return
	}
	operation := strings.TrimPrefix(request.URL.Path, "/v1/")
	if operation != "ensure" && operation != "inspect" && operation != "reset-managed-paths" && operation != "reset-upper" && operation != "unmount" {
		http.NotFound(response, request)
		return
	}
	var input Request
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeServerError(response, http.StatusBadRequest, err)
		return
	}
	if err := ensureJSONEnd(decoder); err != nil {
		writeServerError(response, http.StatusBadRequest, err)
		return
	}
	mount, err := s.paths.Mount(input.InstanceID, input.ReleaseID)
	if err != nil {
		writeServerError(response, http.StatusUnprocessableEntity, err)
		return
	}
	if operation == "reset-managed-paths" {
		for _, managedPath := range input.Paths {
			if !validManagedPath(managedPath) {
				writeServerError(response, http.StatusUnprocessableEntity, errors.New("invalid managed path"))
				return
			}
		}
	}
	switch operation {
	case "ensure":
		err = s.mounter.Ensure(request.Context(), mount)
	case "inspect":
		var status MountStatus
		status, err = s.mounter.Inspect(request.Context(), mount)
		if err == nil {
			response.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(response).Encode(status)
			return
		}
	case "reset-managed-paths":
		err = s.mounter.ResetManagedPaths(request.Context(), mount, input.Paths)
	case "reset-upper":
		err = s.mounter.ResetUpper(request.Context(), mount)
	case "unmount":
		err = s.mounter.Unmount(request.Context(), mount)
	}
	if err != nil {
		writeServerError(response, http.StatusConflict, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("request contains multiple JSON values")
}

func validManagedPath(value string) bool {
	cleaned := path.Clean(strings.ReplaceAll(value, `\`, "/"))
	return value != "" && cleaned != "." && cleaned != ".." && !strings.HasPrefix(cleaned, "../") && !strings.HasPrefix(cleaned, "/") && cleaned == strings.ReplaceAll(value, `\`, "/")
}

func writeServerError(response http.ResponseWriter, status int, err error) {
	http.Error(response, err.Error(), status)
}
