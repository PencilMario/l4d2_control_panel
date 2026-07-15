package traffic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxRequestBody = 64 << 10

type Store interface {
	Register(Session) error
	Stop(instanceID, runID string) error
	Totals(instanceID string) (Totals, bool)
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	} else if client.Timeout <= 0 {
		bounded := *client
		bounded.Timeout = 5 * time.Second
		client = &bounded
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: client}
}

func NewUnixClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socketPath)
		},
		ResponseHeaderTimeout: 3 * time.Second,
	}
	return NewClient("http://socket-proxy", &http.Client{Transport: transport, Timeout: 5 * time.Second})
}

func (c *Client) Register(ctx context.Context, session Session) error {
	return c.doJSON(ctx, http.MethodPut, session.InstanceID, session, nil)
}

func (c *Client) Stop(ctx context.Context, instanceID, runID string) error {
	return c.doJSON(ctx, http.MethodDelete, instanceID, struct {
		RunID string `json:"run_id"`
	}{RunID: runID}, nil)
}

func (c *Client) Totals(ctx context.Context, instanceID string) (Totals, error) {
	var totals Totals
	err := c.doJSON(ctx, http.MethodGet, instanceID, nil, &totals)
	return totals, err
}

func (c *Client) doJSON(ctx context.Context, method, instanceID string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	endpoint := c.baseURL + "/_panel/traffic/" + url.PathEscape(instanceID)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		_, _ = io.Copy(io.Discard, res.Body)
		return fmt.Errorf("traffic proxy returned %s: %s", res.Status, strings.TrimSpace(string(message)))
	}
	if output == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, maxRequestBody)).Decode(output)
}

func NewHandler(store Store, unavailable func() error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable != nil {
			if err := unavailable(); err != nil {
				http.Error(w, "traffic capture unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		const prefix = "/_panel/traffic/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		instanceID, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, prefix))
		if err != nil || !safeID.MatchString(instanceID) {
			http.Error(w, "invalid instance ID", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			var session Session
			if err := decodeStrictJSON(w, r, &session); err != nil {
				return
			}
			if session.InstanceID != instanceID {
				http.Error(w, "body instance ID does not match path", http.StatusConflict)
				return
			}
			if err := store.Register(session); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			var request struct {
				RunID string `json:"run_id"`
			}
			if err := decodeStrictJSON(w, r, &request); err != nil {
				return
			}
			if !validRunID(request.RunID) {
				http.Error(w, "invalid run ID", http.StatusBadRequest)
				return
			}
			err := store.Stop(instanceID, request.RunID)
			if errors.Is(err, ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if errors.Is(err, ErrRunMismatch) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			totals, ok := store.Totals(instanceID)
			if !ok {
				http.Error(w, "traffic session not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(totals)
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
