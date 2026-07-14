package domain

import "time"

type InstanceState string

const (
	StateUninstalled InstanceState = "uninstalled"
	StateInstalling  InstanceState = "installing"
	StateStopped     InstanceState = "stopped"
	StateStarting    InstanceState = "starting"
	StateRunning     InstanceState = "running"
	StateUpdating    InstanceState = "updating"
	StateRollingBack InstanceState = "rolling_back"
	StateFaulted     InstanceState = "faulted"
	StateOrphaned    InstanceState = "orphaned"
)

type Instance struct {
	ID, NodeID, Name, StartMap, GameMode, ExtraArgs, RuntimeImage, PackageVersion string
	GamePort, SourceTVPort, Tickrate, MaxPlayers                                  int
	DesiredState, ActualState                                                     InstanceState
	CreatedAt, UpdatedAt                                                          time.Time
}
