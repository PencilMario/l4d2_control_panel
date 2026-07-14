package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const apiVersion = "/v1.44"

type Engine struct {
	base string
	http *http.Client
}
type Container struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

func (c Container) InstanceID() string { return c.Labels[InstanceLabel] }

type hostConfig struct {
	Binds          []string `json:"Binds"`
	NetworkMode    string   `json:"NetworkMode"`
	Privileged     bool     `json:"Privileged"`
	ReadonlyRootfs bool     `json:"ReadonlyRootfs"`
	SecurityOpt    []string `json:"SecurityOpt"`
}
type createRequest struct {
	Image      string            `json:"Image"`
	Labels     map[string]string `json:"Labels"`
	HostConfig hostConfig        `json:"HostConfig"`
}

func NewEngine(host string) *Engine {
	host = strings.TrimRight(host, "/")
	host = strings.Replace(host, "tcp://", "http://", 1)
	return &Engine{base: host, http: &http.Client{}}
}
func (e *Engine) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	endpoint := e.base + apiVersion + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("docker %s %s: %s: %s", method, path, resp.Status, string(raw))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
func (e *Engine) Create(ctx context.Context, spec ContainerSpec) (string, error) {
	var result struct {
		ID string `json:"Id"`
	}
	request := createRequest{Image: spec.Image, Labels: spec.Labels, HostConfig: hostConfig{Binds: spec.Mounts, NetworkMode: spec.NetworkMode, Privileged: false, ReadonlyRootfs: false, SecurityOpt: []string{"no-new-privileges"}}}
	err := e.do(ctx, http.MethodPost, "/containers/create", url.Values{"name": []string{spec.Name}}, request, &result)
	return result.ID, err
}
func (e *Engine) Start(ctx context.Context, id string) error {
	return e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil, nil, nil)
}
func (e *Engine) Stop(ctx context.Context, id string, seconds int) error {
	return e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/stop", url.Values{"t": []string{strconv.Itoa(seconds)}}, nil, nil)
}
func (e *Engine) Remove(ctx context.Context, id string) error {
	return e.do(ctx, http.MethodDelete, "/containers/"+url.PathEscape(id), url.Values{"force": []string{"false"}, "v": []string{"false"}}, nil, nil)
}
func (e *Engine) CreateSupervisorExec(ctx context.Context, containerID, operation string) (string, error) {
	spec, err := SupervisorExec(containerID, operation)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"Id"`
	}
	body := map[string]any{"AttachStdin": operation == "attach", "AttachStdout": true, "AttachStderr": true, "Tty": true, "Cmd": spec.Args}
	err = e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(containerID)+"/exec", nil, body, &result)
	return result.ID, err
}
func (e *Engine) RunSupervisor(ctx context.Context, containerID, operation string) error {
	id, err := e.CreateSupervisorExec(ctx, containerID, operation)
	if err != nil {
		return err
	}
	return e.do(ctx, http.MethodPost, "/exec/"+url.PathEscape(id)+"/start", nil, map[string]any{"Detach": true, "Tty": true}, nil)
}
func (e *Engine) ListManaged(ctx context.Context) ([]Container, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {ManagedLabel + "=true", RoleLabel + "=game"}})
	var result []Container
	err := e.do(ctx, http.MethodGet, "/containers/json", url.Values{"all": []string{"true"}, "filters": []string{string(filters)}}, nil, &result)
	return result, err
}
