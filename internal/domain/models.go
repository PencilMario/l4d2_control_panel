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
	ID, NodeID, Name, ContainerID, StartMap, GameMode, ExtraArgs, RuntimeImage, PackageVersion string
	GamePort, SourceTVPort, Tickrate, MaxPlayers                                               int
	DesiredState, ActualState                                                                  InstanceState
	CreatedAt, UpdatedAt                                                                       time.Time
}

type JobRecord struct {
	ID, InstanceID, Type, Stage, Message, Status, Error string
	Percent                                             int
	CreatedAt, UpdatedAt                                time.Time
}
type AuditRecord struct {
	ID, Action, Target, Result, Metadata string
	CreatedAt                            time.Time
}
type ScheduledTask struct {
	ID, InstanceID, Type, Cron, Timezone, OnlinePolicy, Payload string
	Enabled                                                     bool
	LastRun, NextRun                                            time.Time
}
