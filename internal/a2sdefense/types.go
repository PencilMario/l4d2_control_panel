package a2sdefense

import (
	"errors"
	"sort"
)

const APIVersion = 1
const PolicyVersion = 1

var ErrInvalidConfig = errors.New("invalid A2S defense config")

type Config struct {
	Version  int   `json:"version"`
	Enabled  bool  `json:"enabled"`
	Ports    []int `json:"ports"`
	Revision int64 `json:"revision"`
}

type Counters struct {
	Info      uint64 `json:"info"`
	Player    uint64 `json:"player"`
	Rules     uint64 `json:"rules"`
	Challenge uint64 `json:"challenge"`
	Other69   uint64 `json:"other_69"`
	Aggregate uint64 `json:"aggregate"`
	Blacklist uint64 `json:"blacklist"`
}

type Status struct {
	Compatible    bool     `json:"compatible"`
	Enabled       bool     `json:"enabled"`
	Revision      int64    `json:"revision"`
	PolicyVersion int      `json:"policy_version"`
	Ports         []int    `json:"ports"`
	Counters      Counters `json:"counters"`
	BlacklistSize int      `json:"blacklist_size"`
	AppliedAt     string   `json:"applied_at,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
}

func NormalizeConfig(input Config) (Config, error) {
	if input.Version != APIVersion || input.Revision < 1 {
		return Config{}, ErrInvalidConfig
	}
	if !input.Enabled && len(input.Ports) != 0 {
		return Config{}, ErrInvalidConfig
	}
	ports := append([]int(nil), input.Ports...)
	sort.Ints(ports)
	for index, port := range ports {
		if port < 1 || port > 65535 || index > 0 && port == ports[index-1] {
			return Config{}, ErrInvalidConfig
		}
	}
	input.Ports = ports
	return input, nil
}
