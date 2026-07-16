package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
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
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
)

const apiVersion = "/v1.44"
const steamCMDEntrypoint = "/home/steam/steamcmd/steamcmd.sh"

type Engine struct {
	base             string
	http             *http.Client
	hijackDial       func(context.Context) (net.Conn, error)
	extraEnv         []string
	steamCredentials func() (string, string)
}

type apiError struct {
	method, path, status, body string
	statusCode                 int
}

func (e *apiError) Error() string {
	return fmt.Sprintf("docker %s %s: %s: %s", e.method, e.path, e.status, e.body)
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
	CPUPercent       float64 `json:"cpu_percent"`
	MemoryBytes      uint64  `json:"memory_bytes"`
	MemoryLimitBytes uint64  `json:"memory_limit_bytes"`
	BlockReadBytes   uint64  `json:"block_read_bytes"`
	BlockWriteBytes  uint64  `json:"block_write_bytes"`
	PIDs             uint64  `json:"pids"`
}
type RuntimeState struct {
	Running   bool
	StartedAt time.Time
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
	runtime, err := e.Runtime(ctx, containerID)
	return runtime.Running, err
}
func (e *Engine) Runtime(ctx context.Context, containerID string) (RuntimeState, error) {
	var result struct {
		State struct {
			Running   bool      `json:"Running"`
			StartedAt time.Time `json:"StartedAt"`
		} `json:"State"`
	}
	if err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(containerID)+"/json", nil, nil, &result); err != nil {
		return RuntimeState{}, err
	}
	return RuntimeState{Running: result.State.Running, StartedAt: result.State.StartedAt}, nil
}

func (e *Engine) ImageSize(ctx context.Context, containerID string) (uint64, error) {
	var container struct {
		Image string `json:"Image"`
	}
	if err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(containerID)+"/json", nil, nil, &container); err != nil {
		return 0, err
	}
	if container.Image == "" {
		return 0, errors.New("container image is empty")
	}
	var image struct {
		Size uint64 `json:"Size"`
	}
	if err := e.do(ctx, http.MethodGet, "/images/"+url.PathEscape(container.Image)+"/json", nil, nil, &image); err != nil {
		return 0, err
	}
	return image.Size, nil
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
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	BlockIO struct {
		ServiceBytes []struct {
			Operation string `json:"op"`
			Value     uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PIDs struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
}

func (e *Engine) Stats(ctx context.Context, containerID string) (ResourceStats, error) {
	var raw statsResponse
	if err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(containerID)+"/stats", url.Values{"stream": []string{"false"}}, nil, &raw); err != nil {
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
	var blockRead, blockWrite uint64
	for _, entry := range raw.BlockIO.ServiceBytes {
		switch {
		case strings.EqualFold(entry.Operation, "read"):
			blockRead += entry.Value
		case strings.EqualFold(entry.Operation, "write"):
			blockWrite += entry.Value
		}
	}
	return ResourceStats{
		CPUPercent:       percent,
		MemoryBytes:      memory,
		MemoryLimitBytes: raw.Memory.Limit,
		BlockReadBytes:   blockRead,
		BlockWriteBytes:  blockWrite,
		PIDs:             raw.PIDs.Current,
	}, nil
}

func (c Container) InstanceID() string { return c.Labels[InstanceLabel] }
func (c Container) Role() string       { return c.Labels[RoleLabel] }

type hostConfig struct {
	Binds          []string `json:"Binds"`
	NetworkMode    string   `json:"NetworkMode"`
	Privileged     bool     `json:"Privileged"`
	ReadonlyRootfs bool     `json:"ReadonlyRootfs"`
	SecurityOpt    []string `json:"SecurityOpt"`
}
type createRequest struct {
	Image      string            `json:"Image"`
	Entrypoint []string          `json:"Entrypoint,omitempty"`
	Cmd        []string          `json:"Cmd,omitempty"`
	User       string            `json:"User,omitempty"`
	Env        []string          `json:"Env,omitempty"`
	Labels     map[string]string `json:"Labels"`
	HostConfig hostConfig        `json:"HostConfig"`
}

func (e *Engine) InstallGame(ctx context.Context, dataRoot string, instance domain.Instance) error {
	login := e.steamLoginArgs()
	command := []string{steamCMDEntrypoint}
	if len(login) == 2 && login[1] == "anonymous" {
		command = append(command, "+@sSteamCmdForcePlatformType", "windows", "+force_install_dir", "/opt/l4d2/game")
		command = append(command, login...)
		command = append(command, "+app_update", "222860", "+@sSteamCmdForcePlatformType", "linux", "+app_update", "222860", "validate", "+quit")
	} else {
		command = append(command, "+@sSteamCmdForcePlatformType", "linux", "+force_install_dir", "/opt/l4d2/game")
		command = append(command, login...)
		command = append(command, "+app_info_update", "1", "+app_update", "222860", "validate", "+quit")
	}
	return e.runMaintenance(ctx, dataRoot, instance, command)
}

func (e *Engine) UpdateGame(ctx context.Context, dataRoot string, instance domain.Instance) error {
	command := []string{steamCMDEntrypoint, "+@sSteamCmdForcePlatformType", "linux", "+force_install_dir", "/opt/l4d2/game"}
	command = append(command, e.steamLoginArgs()...)
	command = append(command, "+app_info_update", "1", "+app_update", "222860", "validate", "+quit")
	return e.runMaintenance(ctx, dataRoot, instance, command)
}

func (e *Engine) runMaintenance(ctx context.Context, dataRoot string, instance domain.Instance, command []string) error {
	maintenance, err := e.maintenanceContainers(ctx, instance.ID)
	if err != nil {
		return err
	}
	if len(maintenance) > 1 {
		return fmt.Errorf("multiple maintenance containers found for instance %s", instance.ID)
	}

	containerID := ""
	if len(maintenance) == 1 {
		containerID = maintenance[0].ID
		if maintenance[0].State == "created" {
			if err := e.Start(ctx, containerID); err != nil {
				return err
			}
		}
	} else {
		body := createRequest{Image: instance.RuntimeImage, Entrypoint: command[:1], Env: e.runtimeEnv(nil), Cmd: command[1:], User: "steam", Labels: map[string]string{ManagedLabel: "true", InstanceLabel: instance.ID, RoleLabel: "maintenance"}, HostConfig: hostConfig{Binds: []string{filepath.Join(dataRoot, "instances", instance.ID, "game") + ":/opt/l4d2/game"}, NetworkMode: "bridge", SecurityOpt: []string{"no-new-privileges"}}}
		var created struct {
			ID string `json:"Id"`
		}
		name := "l4d2-update-" + instance.ID + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
		if err := e.do(ctx, http.MethodPost, "/containers/create", url.Values{"name": []string{name}}, body, &created); err != nil {
			return err
		}
		containerID = created.ID
		if err := e.Start(ctx, containerID); err != nil {
			return err
		}
	}
	logCtx, cancelLogs := context.WithCancel(ctx)
	logErrors := make(chan error, 1)
	go func() {
		logErrors <- e.FollowLogs(logCtx, containerID, func(line string) error {
			jobs.LogContext(ctx, "steamcmd", joblogs.Output, line)
			return nil
		})
	}()

	var result struct {
		StatusCode int `json:"StatusCode"`
	}
	if err := e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(containerID)+"/wait", url.Values{"condition": []string{"not-running"}}, nil, &result); err != nil {
		cancelLogs()
		return err
	}
	select {
	case logErr := <-logErrors:
		if logErr != nil && !errors.Is(logErr, context.Canceled) {
			jobs.LogContext(ctx, "docker", joblogs.Warn, "task log capture failed: "+logErr.Error())
		}
	case <-time.After(5 * time.Second):
		cancelLogs()
		jobs.LogContext(ctx, "docker", joblogs.Warn, "task log capture drain timed out")
		<-logErrors
	}
	cancelLogs()
	if err := e.do(context.WithoutCancel(ctx), http.MethodDelete, "/containers/"+url.PathEscape(containerID), url.Values{"force": []string{"true"}, "v": []string{"false"}}, nil, nil); err != nil {
		return err
	}
	if result.StatusCode != 0 {
		return fmt.Errorf("steamcmd exited with code %d", result.StatusCode)
	}
	return nil
}

func (e *Engine) FollowLogs(ctx context.Context, containerID string, emit func(string) error) error {
	query := url.Values{"stdout": {"1"}, "stderr": {"1"}, "follow": {"1"}, "timestamps": {"1"}, "tail": {"200"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, e.base+apiVersion+"/containers/"+url.PathEscape(containerID)+"/logs?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	response, err := e.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("docker logs: %s: %s", response.Status, string(raw))
	}
	buffers := map[byte]*bytes.Buffer{1: {}, 2: {}}
	emitBuffered := func(stream byte, final bool) error {
		buffer := buffers[stream]
		for {
			raw := buffer.Bytes()
			index := bytes.IndexByte(raw, '\n')
			if index < 0 {
				if final && len(raw) > 0 {
					line := stripDockerLogTimestamp(strings.TrimSuffix(string(raw), "\r"))
					buffer.Reset()
					return emit(line)
				}
				return nil
			}
			line := stripDockerLogTimestamp(strings.TrimSuffix(string(raw[:index]), "\r"))
			buffer.Next(index + 1)
			if err := emit(line); err != nil {
				return err
			}
		}
	}
	header := make([]byte, 8)
	for {
		if _, err := io.ReadFull(response.Body, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return err
		}
		stream := header[0]
		length := binary.BigEndian.Uint32(header[4:])
		if length > 2<<20 {
			return errors.New("docker log frame exceeds limit")
		}
		buffer := buffers[stream]
		if buffer == nil {
			buffer = &bytes.Buffer{}
			buffers[stream] = buffer
		}
		if _, err := io.CopyN(buffer, response.Body, int64(length)); err != nil {
			return err
		}
		if err := emitBuffered(stream, false); err != nil {
			return err
		}
	}
	for _, stream := range []byte{1, 2} {
		if err := emitBuffered(stream, true); err != nil {
			return err
		}
	}
	return nil
}

func stripDockerLogTimestamp(line string) string {
	prefix, remainder, found := strings.Cut(line, " ")
	if !found {
		return line
	}
	if _, err := time.Parse(time.RFC3339Nano, prefix); err != nil {
		return line
	}
	return remainder
}

func (e *Engine) HasMaintenance(ctx context.Context, instanceID string) (bool, error) {
	containers, err := e.maintenanceContainers(ctx, instanceID)
	return len(containers) > 0, err
}

func (e *Engine) maintenanceContainers(ctx context.Context, instanceID string) ([]Container, error) {
	containers, err := e.ListManaged(ctx)
	if err != nil {
		return nil, err
	}
	maintenance := make([]Container, 0, 1)
	for _, container := range containers {
		if container.InstanceID() == instanceID && container.Role() == "maintenance" {
			maintenance = append(maintenance, container)
		}
	}
	return maintenance, nil
}

func NewEngine(host string, options ...EngineOption) *Engine {
	client := &http.Client{}
	var hijackDial func(context.Context) (net.Conn, error)
	if strings.HasPrefix(host, "unix://") {
		socketPath := strings.TrimPrefix(host, "unix://")
		hijackDial = func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}
		client.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return hijackDial(ctx)
		}}
		host = "http://docker"
	}
	host = strings.TrimRight(host, "/")
	host = strings.Replace(host, "tcp://", "http://", 1)
	if hijackDial == nil {
		hijackDial = func(ctx context.Context) (net.Conn, error) {
			parsed, err := url.Parse(host)
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
			return (&net.Dialer{}).DialContext(ctx, "tcp", address)
		}
	}
	engine := &Engine{base: host, http: client, hijackDial: hijackDial}
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
		return &apiError{method: method, path: path, status: resp.Status, body: string(raw), statusCode: resp.StatusCode}
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
	err := e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/stop", url.Values{"t": []string{strconv.Itoa(seconds)}}, nil, nil)
	var responseErr *apiError
	if errors.As(err, &responseErr) && responseErr.statusCode == http.StatusNotModified {
		return nil
	}
	return err
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
	conn, err := e.hijackDial(ctx)
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
	identity := strings.ReplaceAll(uuid.NewString(), "-", "")
	startMarker := "L4D2_PANEL_START_" + identity
	endMarker := "L4D2_PANEL_END_" + identity
	if deadline, ok := stream.(interface{ SetDeadline(time.Time) error }); ok {
		_ = deadline.SetDeadline(time.Now().Add(4 * time.Second))
	}
	if _, err := io.WriteString(stream, "echo "+startMarker+"\n"+command+"\necho "+endMarker+"\n"); err != nil {
		return "", err
	}
	var output bytes.Buffer
	buffer := make([]byte, 16*1024)
	for output.Len() < 2*1024*1024 {
		n, readErr := stream.Read(buffer)
		if n > 0 {
			output.Write(buffer[:n])
			if response, ok := extractConsoleResponse(output.String(), startMarker, endMarker); ok {
				return response, nil
			}
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return "", errors.New("console response exceeded limit")
}

func extractConsoleResponse(output, startMarker, endMarker string) (string, bool) {
	started := false
	response := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch strings.TrimSpace(line) {
		case startMarker:
			started = true
			response = response[:0]
		case endMarker:
			if started {
				return strings.TrimSpace(strings.Join(response, "\n")), true
			}
		default:
			if started {
				response = append(response, line)
			}
		}
	}
	return "", false
}
func (e *Engine) ListManaged(ctx context.Context) ([]Container, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {ManagedLabel + "=true"}})
	var result []Container
	err := e.do(ctx, http.MethodGet, "/containers/json", url.Values{"all": []string{"true"}, "filters": []string{string(filters)}}, nil, &result)
	return result, err
}
