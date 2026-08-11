package crashreports

import (
	"errors"
	"strconv"
	"strings"
)

var ErrInvalidCrashSignature = errors.New("invalid crash signature")

type CrashSignature struct {
	Version          int      `json:"version"`
	Timestamp        string   `json:"timestamp,omitempty"`
	Platform         string   `json:"platform,omitempty"`
	Architecture     string   `json:"architecture,omitempty"`
	Crashed          string   `json:"crashed,omitempty"`
	CrashReason      string   `json:"crash_reason,omitempty"`
	CrashAddress     string   `json:"crash_address,omitempty"`
	RequestingThread int      `json:"requesting_thread,omitempty"`
	Modules          []Module `json:"modules,omitempty"`
	Frames           []Frame  `json:"frames,omitempty"`
}

type Module struct {
	Index           int    `json:"index"`
	DebugFile       string `json:"debug_file"`
	DebugIdentifier string `json:"debug_identifier"`
	CodeIdentifier  string `json:"code_identifier,omitempty"`
	Platform        string `json:"platform,omitempty"`
	Architecture    string `json:"architecture,omitempty"`
	Decision        string `json:"decision,omitempty"`
	SymbolArtifact  string `json:"symbol_artifact,omitempty"`
	BinaryArtifact  string `json:"binary_artifact,omitempty"`
}

type Frame struct {
	ModuleIndex int    `json:"module_index"`
	Offset      string `json:"offset"`
}

func ParseCrashSignature(raw string) (CrashSignature, error) {
	if raw == "" || len(raw) > MaxCrashSignatureBytes {
		return CrashSignature{}, ErrInvalidCrashSignature
	}
	parts := strings.Split(raw, "|")
	if len(parts) < 6 {
		return CrashSignature{}, ErrInvalidCrashSignature
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil || version != 2 {
		return CrashSignature{}, ErrInvalidCrashSignature
	}
	thread := 0
	moduleStart := 6
	if len(parts) == 7 {
		return CrashSignature{}, ErrInvalidCrashSignature
	}
	if len(parts) >= 8 {
		thread, err = strconv.Atoi(parts[7])
		if err != nil || thread < 0 {
			return CrashSignature{}, ErrInvalidCrashSignature
		}
		moduleStart = 8
	}
	result := CrashSignature{
		Version:          version,
		Timestamp:        parts[1],
		Platform:         strings.ToLower(parts[2]),
		Architecture:     strings.ToLower(parts[3]),
		Crashed:          parts[4],
		CrashReason:      parts[5],
		RequestingThread: thread,
		Modules:          []Module{},
		Frames:           []Frame{},
	}
	if len(parts) >= 7 {
		result.CrashAddress = parts[6]
	}
	if result.Platform == "" || result.Architecture == "" {
		return CrashSignature{}, ErrInvalidCrashSignature
	}
	for index := moduleStart; index < len(parts); {
		switch parts[index] {
		case "M":
			if index+1 >= len(parts) || parts[index+1] == "" || len(result.Modules) >= MaxModuleCount {
				return CrashSignature{}, ErrInvalidCrashSignature
			}
			module := Module{Index: len(result.Modules), DebugFile: parts[index+1], Platform: result.Platform, Architecture: result.Architecture}
			index += 2
			// Older Accelerator-compatible senders used M|name without a debug
			// identifier. Keep that input as an unknown module for compatibility.
			if index < len(parts) && parts[index] != "M" && parts[index] != "F" && parts[index] != "" {
				module.DebugIdentifier = parts[index]
				index++
			}
			result.Modules = append(result.Modules, module)
		case "F":
			if index+2 >= len(parts) || parts[index+1] == "" || parts[index+2] == "" {
				return CrashSignature{}, ErrInvalidCrashSignature
			}
			moduleIndex, parseErr := strconv.Atoi(parts[index+1])
			if parseErr != nil || moduleIndex < -1 || moduleIndex >= len(result.Modules) {
				return CrashSignature{}, ErrInvalidCrashSignature
			}
			result.Frames = append(result.Frames, Frame{ModuleIndex: moduleIndex, Offset: parts[index+2]})
			index += 3
		default:
			return CrashSignature{}, ErrInvalidCrashSignature
		}
	}
	return result, nil
}
