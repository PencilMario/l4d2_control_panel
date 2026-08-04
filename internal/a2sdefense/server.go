package a2sdefense

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type Server struct {
	firewall Firewall
}

func NewServer(firewall Firewall) *Server {
	return &Server{firewall: firewall}
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
