package crashreports

import (
	"context"
	"errors"
	"io"
	"time"
)

const (
	MinRetentionDays = 1
	MaxRetentionDays = 3650

	MaxMinidumpBytes       int64 = 128 << 20
	MaxMetadataBytes       int64 = 4 << 20
	MaxSymbolBytes         int64 = 32 << 20
	MaxBinaryBytes         int64 = 256 << 20
	MaxRequestBytes        int64 = 512 << 20
	MaxCrashSignatureBytes       = 256 << 10
	MaxModuleCount               = 1024
	PendingRetention             = 24 * time.Hour
)

var (
	ErrInvalidConfig          = errors.New("invalid crash report configuration")
	ErrNotFound               = errors.New("crash report not found")
	ErrInvalidFileKind        = errors.New("invalid crash report file kind")
	ErrInvalidMinidump        = errors.New("invalid minidump")
	ErrInvalidSymbol          = errors.New("invalid symbol")
	ErrInvalidBinary          = errors.New("invalid binary")
	ErrInstanceNotAllowed     = errors.New("crash report instance is not allowed")
	ErrMetadataRequired       = errors.New("metadata is required")
	ErrMinidumpTooLarge       = errors.New("minidump exceeds size limit")
	ErrMetadataTooLarge       = errors.New("metadata exceeds size limit")
	ErrSymbolTooLarge         = errors.New("symbol exceeds size limit")
	ErrBinaryTooLarge         = errors.New("binary exceeds size limit")
	ErrCrashSignatureTooLarge = errors.New("crash signature exceeds size limit")
	ErrTooManyModules         = errors.New("crash signature contains too many modules")
)

type Config struct {
	Root                 string
	Token                string
	Now                  func() time.Time
	AuthorizeInstance    InstanceAuthorizer
	ResolveInstance      InstanceResolver
	ResolveContainerID   func(context.Context, string) (string, error)
	EnqueueAnalysis      func(context.Context, Report) error
	AnalysisEnqueueError func(error)
}

type InstanceAuthorizer func(context.Context, string, string) error
type InstanceResolver func(context.Context, string, string) (string, error)

type PreSubmitInput struct {
	UserID           string
	ExtensionVersion string
	ServerID         string
	CrashSignature   string
	InstanceID       string
}

type UploadInput struct {
	UserID           string
	GameDirectory    string
	ExtensionVersion string
	ServerID         string
	PresubmitToken   string
	InstanceID       string
	Minidump         io.Reader
	Metadata         io.Reader
}

type SymbolInput struct {
	UserID           string
	ExtensionVersion string
	ServerID         string
	PresubmitToken   string
	InstanceID       string
	Platform         string
	Architecture     string
	DebugIdentifier  string
	CodeIdentifier   string
	DebugFile        string
	Symbol           io.Reader
}

type BinaryInput struct {
	UserID           string
	ExtensionVersion string
	ServerID         string
	PresubmitToken   string
	InstanceID       string
	Platform         string
	Architecture     string
	DebugIdentifier  string
	CodeIdentifier   string
	CodeFileName     string
	CodeFile         io.Reader
}

type PendingSubmission struct {
	Token       string         `json:"token"`
	CreatedAt   time.Time      `json:"created_at"`
	ContainerID string         `json:"container_id,omitempty"`
	Input       PreSubmitInput `json:"input"`
}

type Report struct {
	ID               string          `json:"id"`
	InstanceID       string          `json:"instance_id,omitempty"`
	ContainerID      string          `json:"container_id,omitempty"`
	ReceivedAt       time.Time       `json:"received_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	MinidumpSize     int64           `json:"minidump_size"`
	MetadataSize     int64           `json:"metadata_size"`
	SHA256           string          `json:"sha256"`
	UserID           string          `json:"user_id,omitempty"`
	GameDirectory    string          `json:"game_directory,omitempty"`
	ExtensionVersion string          `json:"extension_version,omitempty"`
	ServerID         string          `json:"server_id,omitempty"`
	CrashSignature   string          `json:"crash_signature,omitempty"`
	PresubmitToken   string          `json:"presubmit_token,omitempty"`
	ParsedSignature  *CrashSignature `json:"parsed_signature,omitempty"`
	Modules          []Module        `json:"modules,omitempty"`
	StackwalkStatus  AnalysisStatus  `json:"stackwalk_status,omitempty"`
	StackwalkError   string          `json:"stackwalk_error,omitempty"`
	StackwalkTool    string          `json:"stackwalk_tool,omitempty"`
	StackwalkAt      time.Time       `json:"stackwalk_at,omitempty"`
	AIStatus         AnalysisStatus  `json:"ai_status,omitempty"`
	AIError          string          `json:"ai_error,omitempty"`
	AIModel          string          `json:"ai_model,omitempty"`
	AIInputSHA256    string          `json:"ai_input_sha256,omitempty"`
	AIAnalysis       string          `json:"ai_analysis,omitempty"`
	AIStartedAt      time.Time       `json:"ai_started_at,omitempty"`
	AICompletedAt    time.Time       `json:"ai_completed_at,omitempty"`
}

type AnalysisStatus string

const (
	AnalysisStatusQueued       AnalysisStatus = "queued"
	AnalysisStatusRunning      AnalysisStatus = "running"
	AnalysisStatusSucceeded    AnalysisStatus = "succeeded"
	AnalysisStatusFailed       AnalysisStatus = "failed"
	AnalysisStatusUnconfigured AnalysisStatus = "unconfigured"
)

type StackwalkInput struct {
	DumpPath   string
	SymbolRoot string
}

type StackwalkUpdate struct {
	Status AnalysisStatus
	Error  string
	Text   string
	Tool   string
}

type AIAnalysisUpdate struct {
	Status      AnalysisStatus
	Error       string
	Model       string
	InputSHA256 string
	Text        string
	StartedAt   time.Time
	CompletedAt time.Time
}

type AnalysisRecovery struct {
	ID        string
	RequestAI bool
}

type CleanupResult struct {
	ReportsRemoved   int   `json:"reports_removed"`
	PendingRemoved   int   `json:"pending_removed"`
	ArtifactsRemoved int   `json:"artifacts_removed"`
	BytesReleased    int64 `json:"bytes_released"`
}

type FileKind string

const (
	FileKindMinidump  FileKind = "minidump"
	FileKindMetadata  FileKind = "metadata"
	FileKindStackwalk FileKind = "stackwalk"
	FileKindAI        FileKind = "ai"
)
