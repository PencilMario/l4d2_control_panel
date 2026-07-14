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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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
