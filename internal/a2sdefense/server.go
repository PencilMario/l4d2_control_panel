package a2sdefense

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type EventReader interface {
	Batch(string, uint64) EventBatch
}

type Server struct {
	firewall Firewall
	events   EventReader
}

func NewServer(firewall Firewall, events ...EventReader) *Server {
	server := &Server{firewall: firewall}
	if len(events) > 0 {
		server.events = events[0]
	}
	return server
}

func (s *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/v1/config":
		if request.Method != http.MethodPut {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var config Config
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			if err == nil {
				err = errors.New("request contains multiple JSON values")
			}
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		status, err := s.firewall.Apply(request.Context(), config)
		if err != nil {
			code := http.StatusServiceUnavailable
			if errors.Is(err, ErrStaleRevision) {
				code = http.StatusConflict
			} else if errors.Is(err, ErrInvalidConfig) {
				code = http.StatusUnprocessableEntity
			}
			http.Error(response, err.Error(), code)
			return
		}
		writeJSON(response, status)
	case "/v1/events":
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.events == nil {
			http.Error(response, "events unavailable", http.StatusServiceUnavailable)
			return
		}
		query := request.URL.Query()
		for key, values := range query {
			if (key != "boot" && key != "after") || len(values) != 1 {
				http.Error(response, "invalid event cursor", http.StatusBadRequest)
				return
			}
		}
		boot := query.Get("boot")
		if len(boot) > 64 || strings.ContainsAny(boot, "\x00\r\n") {
			http.Error(response, "invalid event boot", http.StatusBadRequest)
			return
		}
		after := uint64(0)
		if raw, exists := query["after"]; exists {
			parsed, err := strconv.ParseUint(raw[0], 10, 64)
			if err != nil {
				http.Error(response, "invalid event sequence", http.StatusBadRequest)
				return
			}
			after = parsed
		}
		writeJSON(response, s.events.Batch(boot, after))
	case "/v1/status":
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status, err := s.firewall.Status(request.Context())
		if err != nil {
			http.Error(response, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeJSON(response, status)
	default:
		http.NotFound(response, request)
	}
}

func writeJSON(response http.ResponseWriter, value any) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(value)
}
