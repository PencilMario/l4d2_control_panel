package overlayfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
	}}
	return &Client{httpClient: &http.Client{Transport: transport, Timeout: 30 * time.Second}}
}

func (c *Client) Preflight(ctx context.Context) error {
	return c.post(ctx, "/v1/preflight", Request{}, nil)
}

func (c *Client) Ensure(ctx context.Context, instanceID, releaseID string) error {
	return c.post(ctx, "/v1/ensure", Request{InstanceID: instanceID, ReleaseID: releaseID}, nil)
}

func (c *Client) Inspect(ctx context.Context, instanceID, releaseID string) (MountStatus, error) {
	var status MountStatus
	err := c.post(ctx, "/v1/inspect", Request{InstanceID: instanceID, ReleaseID: releaseID}, &status)
	return status, err
}

func (c *Client) ResetManagedPaths(ctx context.Context, instanceID, releaseID string, paths []string) error {
	return c.post(ctx, "/v1/reset-managed-paths", Request{InstanceID: instanceID, ReleaseID: releaseID, Paths: paths}, nil)
}

func (c *Client) ResetUpper(ctx context.Context, instanceID, releaseID string) error {
	return c.post(ctx, "/v1/reset-upper", Request{InstanceID: instanceID, ReleaseID: releaseID}, nil)
}

func (c *Client) Unmount(ctx context.Context, instanceID, releaseID string) error {
	return c.post(ctx, "/v1/unmount", Request{InstanceID: instanceID, ReleaseID: releaseID}, nil)
}

func (c *Client) post(ctx context.Context, endpoint string, input Request, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://overlay-helper"+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("overlay helper returned %s", response.Status)
	}
	if output != nil {
		return json.NewDecoder(response.Body).Decode(output)
	}
	return nil
}
