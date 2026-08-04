package a2sdefense

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
	}}
	return &Client{httpClient: &http.Client{Transport: transport, Timeout: 15 * time.Second}}
}

func (c *Client) Apply(ctx context.Context, config Config) (Status, error) {
	body, err := json.Marshal(config)
	if err != nil {
		return Status{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://a2s-defense-helper/v1/config", bytes.NewReader(body))
	if err != nil {
		return Status{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request)
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://a2s-defense-helper/v1/status", nil)
	if err != nil {
		return Status{}, err
	}
	return c.do(request)
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
