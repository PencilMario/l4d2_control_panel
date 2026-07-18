package overlayfs

import (
	"errors"
	"path/filepath"
	"regexp"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Mount struct {
	InstanceID string `json:"instance_id"`
	ReleaseID  string `json:"release_id"`
	Lower      string `json:"lower"`
	Upper      string `json:"upper"`
	Work       string `json:"work"`
	Merged     string `json:"merged"`
}

type Paths struct {
	Root string
}

func (p Paths) Mount(instanceID, releaseID string) (Mount, error) {
	if !validIdentifier(instanceID) || !validIdentifier(releaseID) {
		return Mount{}, errors.New("invalid instance or release identifier")
	}
	root, err := filepath.Abs(p.Root)
	if err != nil {
		return Mount{}, err
	}
	overlay := filepath.Join(root, "instances", instanceID, "overlay")
	return Mount{
		InstanceID: instanceID,
		ReleaseID:  releaseID,
		Lower:      filepath.Join(root, "game", "releases", releaseID),
		Upper:      filepath.Join(overlay, "upper"),
		Work:       filepath.Join(overlay, "work"),
		Merged:     filepath.Join(overlay, "merged"),
	}, nil
}

func validIdentifier(value string) bool {
	return value != "." && value != ".." && identifierPattern.MatchString(value)
}
