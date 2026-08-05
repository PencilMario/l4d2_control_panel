package a2sdefense

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
	}}
	return &Client{httpClient: &http.Client{Transport: transport, Timeout: 15 * time.Second}, baseURL: "http://a2s-defense-helper"}
}

func (c *Client) Apply(ctx context.Context, config Config) (Status, error) {
	body, err := json.Marshal(config)
	if err != nil {
		return Status{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.endpoint("/v1/config"), bytes.NewReader(body))
	if err != nil {
		return Status{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request)
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/status"), nil)
	if err != nil {
		return Status{}, err
	}
	return c.do(request)
}

func (c *Client) Events(ctx context.Context, bootID string, after uint64) (EventBatch, error) {
	query := url.Values{"after": []string{strconv.FormatUint(after, 10)}}
	if bootID != "" {
		query.Set("boot", bootID)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/events")+"?"+query.Encode(), nil)
	if err != nil {
		return EventBatch{}, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return EventBatch{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return EventBatch{}, fmt.Errorf("A2S defense helper returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var batch EventBatch
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&batch); err != nil {
		return EventBatch{}, err
	}
	return batch, nil
}

func (c *Client) endpoint(path string) string {
	if c.baseURL == "" {
		return "http://a2s-defense-helper" + path
	}
	return c.baseURL + path
}

func (c *Client) do(request *http.Request) (Status, error) {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return Status{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return Status{}, fmt.Errorf("A2S defense helper returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var status Status
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status); err != nil {
		return Status{}, err
	}
	return status, nil
}
