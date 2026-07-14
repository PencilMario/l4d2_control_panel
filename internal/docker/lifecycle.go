package docker

import (
	"errors"
	"path/filepath"
	"strconv"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

const ManagedLabel = "io.l4d2-panel.managed"
const InstanceLabel = "io.l4d2-panel.instance-id"
const RoleLabel = "io.l4d2-panel.role"

type ContainerSpec struct {
	Name, Image, NetworkMode string
	Labels                   map[string]string
	Mounts                   []string
	Env                      []string
}
type ExecSpec struct {
	ContainerID string
	Args        []string
}

func BuildContainerSpec(root string, v domain.Instance) ContainerSpec {
	base := filepath.Join(root, "instances", v.ID)
	return ContainerSpec{Name: "l4d2-" + v.ID, Image: v.RuntimeImage, NetworkMode: "host", Labels: map[string]string{ManagedLabel: "true", InstanceLabel: v.ID, RoleLabel: "game"}, Mounts: []string{filepath.Join(base, "game") + ":/opt/l4d2/game", filepath.Join(base, "private") + ":/opt/l4d2/private", filepath.Join(root, "shared-vpk") + ":/opt/l4d2/shared-vpk:ro"}, Env: []string{"SRCDS_PORT=" + strconv.Itoa(v.GamePort), "SRCDS_MAP=" + v.StartMap, "SRCDS_MODE=" + v.GameMode, "SRCDS_TICKRATE=" + strconv.Itoa(v.Tickrate), "SRCDS_MAXPLAYERS=" + strconv.Itoa(v.MaxPlayers), "SRCDS_EXTRA_ARGS=" + v.ExtraArgs}}
}
func SupervisorExec(containerID, operation string) (ExecSpec, error) {
	allowed := map[string][]string{"attach": {"l4d2-supervisor", "attach"}, "status": {"l4d2-supervisor", "status", "--json"}, "stop": {"l4d2-supervisor", "stop"}}
	args, ok := allowed[operation]
	if !ok {
		return ExecSpec{}, errors.New("unsupported supervisor operation")
	}
	return ExecSpec{ContainerID: containerID, Args: args}, nil
}
