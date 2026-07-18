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
	ID, NodeID, Name, ContainerID, StartMap, GameMode string
	ExtraArgs                                         string `json:"extra_args"`
	RuntimeImage                                      string
	PackageVersion                                    string `json:"applied_package_id"`
	SelectedPackageID                                 string `json:"package_id"`
	GamePort                                          int
	SourceTVPort                                      int   `json:"sourcetv_port"`
	PluginPorts                                       []int `json:"plugin_ports"`
	Tickrate, MaxPlayers                              int
	DesiredState, ActualState                         InstanceState
	CreatedAt, UpdatedAt                              time.Time
}

type VPKRestart struct {
	InstanceID, ContainerID, PublicationID, Status string
	Failures                                       int
	CreatedAt, UpdatedAt                           time.Time
}

type JobRecord struct {
	ID, InstanceID, Type, Stage, Message, Status, Error string
	Percent                                             int
	CreatedAt, UpdatedAt                                time.Time
	StartedAt, FinishedAt                               *time.Time
}
type JobEvent struct {
	ID                          int64
	JobID, Kind, Stage, Message string
	Percent                     int
	CreatedAt                   time.Time
}
type AuditRecord struct {
	ID, Action, Target, Result, Metadata string
	CreatedAt                            time.Time
}
type GitHubSource struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Repository   string    `json:"repository"`
	AssetPattern string    `json:"asset_pattern"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type ScheduledTask struct {
	ID           string    `json:"id"`
	InstanceID   string    `json:"instance_id"`
	Type         string    `json:"type"`
	Cron         string    `json:"cron"`
	Timezone     string    `json:"timezone"`
	OnlinePolicy string    `json:"online_policy"`
	Payload      string    `json:"payload"`
	Enabled      bool      `json:"enabled"`
	LastRun      time.Time `json:"last_run"`
	NextRun      time.Time `json:"next_run"`
}

type SharedGameState struct {
	ActiveReleaseID   string    `json:"active_release_id"`
	PreviousReleaseID string    `json:"previous_release_id"`
	MigrationState    string    `json:"migration_state"`
	OperationID       string    `json:"operation_id"`
	OperationStage    string    `json:"operation_stage"`
	UpdatedAt         time.Time `json:"updated_at"`
	Version           string    `json:"version,omitempty"`
	Path              string    `json:"path,omitempty"`
}
