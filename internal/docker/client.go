package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

const apiVersion = "/v1.44"

type Engine struct {
	base             string
	http             *http.Client
	extraEnv         []string
	steamCredentials func() (string, string)
}
type EngineOption func(*Engine)

func WithDownloadProxy(proxy string) EngineOption {
	return func(e *Engine) {
		if proxy != "" {
			e.extraEnv = []string{"HTTP_PROXY=" + proxy, "HTTPS_PROXY=" + proxy}
		}
	}
}
func WithSteamCredentials(provider func() (string, string)) EngineOption {
	return func(e *Engine) { e.steamCredentials = provider }
}
func (e *Engine) runtimeEnv(base []string) []string {
	result := append(append([]string{}, base...), e.extraEnv...)
	if e.steamCredentials != nil {
		username, password := e.steamCredentials()
		if username != "" {
			result = append(result, "STEAM_USERNAME="+username, "STEAM_PASSWORD="+password)
		}
	}
	return result
}
func (e *Engine) steamLoginArgs() []string {
	if e.steamCredentials != nil {
		username, password := e.steamCredentials()
		if username != "" {
			return []string{"+login", username, password}
		}
	}
	return []string{"+login", "anonymous"}
}

type Container struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}
type ResourceStats struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
}
type Info struct {
	ServerVersion     string `json:"ServerVersion"`
	ContainersRunning int    `json:"ContainersRunning"`
}

func (e *Engine) Info(ctx context.Context) (Info, error) {
	var info Info
	err := e.do(ctx, http.MethodGet, "/info", nil, nil, &info)
	return info, err
}
func (e *Engine) Running(ctx context.Context, containerID string) (bool, error) {
	var result struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
	}
	if err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(containerID)+"/json", nil, nil, &result); err != nil {
		return false, err
	}
	return result.State.Running, nil
}

type statsResponse struct {
	CPUStats struct {
		CPUUsage struct {
			Total  uint64   `json:"total_usage"`
			PerCPU []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		System uint64 `json:"system_cpu_usage"`
		Online int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			Total uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		System uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	Memory struct {
		Usage uint64            `json:"usage"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}

func (e *Engine) Stats(ctx context.Context, containerID string) (ResourceStats, error) {
	var raw statsResponse
	if err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(containerID)+"/stats", url.Values{"stream": []string{"false"}, "one-shot": []string{"true"}}, nil, &raw); err != nil {
		return ResourceStats{}, err
	}
	cpuDelta := raw.CPUStats.CPUUsage.Total - raw.PreCPUStats.CPUUsage.Total
	systemDelta := raw.CPUStats.System - raw.PreCPUStats.System
	cpus := raw.CPUStats.Online
	if cpus == 0 {
		cpus = len(raw.CPUStats.CPUUsage.PerCPU)
	}
	percent := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		percent = float64(cpuDelta) / float64(systemDelta) * float64(cpus) * 100
	}
	memory := raw.Memory.Usage
	if cache := raw.Memory.Stats["cache"]; cache < memory {
		memory -= cache
	}
	return ResourceStats{CPUPercent: percent, MemoryBytes: memory}, nil
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
	Cmd        []string          `json:"Cmd,omitempty"`
	User       string            `json:"User,omitempty"`
	Env        []string          `json:"Env,omitempty"`
	Labels     map[string]string `json:"Labels"`
	HostConfig hostConfig        `json:"HostConfig"`
}

func (e *Engine) UpdateGame(ctx context.Context, dataRoot string, instance domain.Instance) error {
	command := []string{"steamcmd", "+@sSteamCmdForcePlatformType", "linux", "+force_install_dir", "/opt/l4d2/game"}
	command = append(command, e.steamLoginArgs()...)
	command = append(command, "+app_info_update", "1", "+app_update", "222860", "validate", "+quit")
	body := createRequest{Image: instance.RuntimeImage, Env: e.runtimeEnv(nil), Cmd: command, User: "steam", Labels: map[string]string{ManagedLabel: "true", InstanceLabel: instance.ID, RoleLabel: "maintenance"}, HostConfig: hostConfig{Binds: []string{filepath.Join(dataRoot, "instances", instance.ID, "game") + ":/opt/l4d2/game"}, NetworkMode: "bridge", SecurityOpt: []string{"no-new-privileges"}}}
	var created struct {
		ID string `json:"Id"`
	}
	name := "l4d2-update-" + instance.ID + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	if err := e.do(ctx, http.MethodPost, "/containers/create", url.Values{"name": []string{name}}, body, &created); err != nil {
		return err
	}
	defer func() {
		_ = e.do(context.WithoutCancel(ctx), http.MethodDelete, "/containers/"+url.PathEscape(created.ID), url.Values{"force": []string{"true"}, "v": []string{"false"}}, nil, nil)
	}()
	if err := e.Start(ctx, created.ID); err != nil {
		return err
	}
	var result struct {
		StatusCode int `json:"StatusCode"`
	}
	if err := e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(created.ID)+"/wait", url.Values{"condition": []string{"not-running"}}, nil, &result); err != nil {
		return err
	}
	if result.StatusCode != 0 {
		return fmt.Errorf("steamcmd exited with code %d", result.StatusCode)
	}
	return nil
}

func NewEngine(host string, options ...EngineOption) *Engine {
	host = strings.TrimRight(host, "/")
	host = strings.Replace(host, "tcp://", "http://", 1)
	engine := &Engine{base: host, http: &http.Client{}}
	for _, option := range options {
		option(engine)
	}
	return engine
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
	request := createRequest{Image: spec.Image, Env: e.runtimeEnv(spec.Env), Labels: spec.Labels, HostConfig: hostConfig{Binds: spec.Mounts, NetworkMode: spec.NetworkMode, Privileged: false, ReadonlyRootfs: false, SecurityOpt: []string{"no-new-privileges"}}}
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

type attachStream struct {
	net.Conn
	reader *bufio.Reader
}

func (s *attachStream) Read(p []byte) (int, error) { return s.reader.Read(p) }
func (e *Engine) AttachSupervisor(ctx context.Context, containerID string) (io.ReadWriteCloser, error) {
	execID, err := e.CreateSupervisorExec(ctx, containerID, "attach")
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(e.base)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" {
		return nil, errors.New("docker attach requires an http socket proxy")
	}
	address := parsed.Host
	if !strings.Contains(address, ":") {
		address += ":80"
	}
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	body := strings.NewReader(`{"Detach":false,"Tty":true}`)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, e.base+apiVersion+"/exec/"+url.PathEscape(execID)+"/start", body)
	if err != nil {
		conn.Close()
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "tcp")
	if err := request.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		conn.Close()
		return nil, fmt.Errorf("docker attach: %s: %s", response.Status, string(raw))
	}
	return &attachStream{Conn: conn, reader: reader}, nil
}

var playerCommandPattern = regexp.MustCompile(`^(kickid [1-9][0-9]*|banid (0|[1-9][0-9]*) [1-9][0-9]* kick; writeid)$`)

func (e *Engine) Status(ctx context.Context, containerID string) (string, error) {
	return e.consoleRoundTrip(ctx, containerID, "status")
}
func (e *Engine) PlayerCommand(ctx context.Context, containerID, command string) error {
	if !playerCommandPattern.MatchString(command) {
		return errors.New("console command is not an approved player operation")
	}
	_, err := e.consoleRoundTrip(ctx, containerID, command)
	return err
}
func (e *Engine) consoleRoundTrip(ctx context.Context, containerID, command string) (string, error) {
	stream, err := e.AttachSupervisor(ctx, containerID)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	marker := "L4D2_PANEL_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if deadline, ok := stream.(interface{ SetDeadline(time.Time) error }); ok {
		_ = deadline.SetDeadline(time.Now().Add(4 * time.Second))
	}
	if _, err := io.WriteString(stream, command+"\necho "+marker+"\n"); err != nil {
		return "", err
	}
	var output bytes.Buffer
	buffer := make([]byte, 16*1024)
	for output.Len() < 2*1024*1024 {
		n, readErr := stream.Read(buffer)
		if n > 0 {
			output.Write(buffer[:n])
			if strings.Contains(output.String(), marker) {
				return output.String(), nil
			}
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return "", errors.New("console response exceeded limit")
}
func (e *Engine) ListManaged(ctx context.Context) ([]Container, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {ManagedLabel + "=true", RoleLabel + "=game"}})
	var result []Container
	err := e.do(ctx, http.MethodGet, "/containers/json", url.Values{"all": []string{"true"}, "filters": []string{string(filters)}}, nil, &result)
	return result, err
}
