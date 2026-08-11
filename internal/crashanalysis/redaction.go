package crashanalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
)

const maxRedactedTextBytes = 256 << 10

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^(\s*(?:commandline|cmdline|command)\s*=\s*).*$`),
	regexp.MustCompile(`(?i)(token=)[^&\s]+`),
	regexp.MustCompile(`(?i)(?:STEAM_[0-9]:[0-9]:[0-9]+|\[U:[0-9]+:[0-9]+\]|765611[0-9]{10})`),
	regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`),
	regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?::[0-9]{1,5})?\b`),
	regexp.MustCompile(`(?i)\b(?:[0-9a-f]{0,4}:){2,}[0-9a-f]{0,4}(?::[0-9]{1,5})?\b`),
	regexp.MustCompile(`[A-Za-z]:[\\/][^\s"'<>]+`),
	regexp.MustCompile(`(^|[\s=(])/(?:[^\s"'<>]+)`),
	regexp.MustCompile(`(?im)^(\s*(?:serverid|server_id|userid|user_id)\s*=\s*).*$`),
}

type AIInput struct {
	Body   []byte
	SHA256 string
}

type aiModule struct {
	DebugFile       string `json:"debug_file,omitempty"`
	DebugIdentifier string `json:"debug_identifier,omitempty"`
	CodeIdentifier  string `json:"code_identifier,omitempty"`
	Platform        string `json:"platform,omitempty"`
	Architecture    string `json:"architecture,omitempty"`
	Decision        string `json:"decision,omitempty"`
}

type aiPayload struct {
	Platform         string     `json:"platform,omitempty"`
	Architecture     string     `json:"architecture,omitempty"`
	Crashed          string     `json:"crashed,omitempty"`
	CrashReason      string     `json:"crash_reason,omitempty"`
	CrashAddress     string     `json:"crash_address,omitempty"`
	RequestingThread int        `json:"requesting_thread,omitempty"`
	Modules          []aiModule `json:"modules,omitempty"`
	Metadata         string     `json:"metadata,omitempty"`
	Stackwalk        string     `json:"stackwalk,omitempty"`
}

func Redact(value string) string {
	for _, pattern := range redactionPatterns {
		replacement := "<redacted>"
		if pattern == redactionPatterns[0] {
			replacement = "$1<redacted-command>"
		} else if pattern == redactionPatterns[1] {
			replacement = "$1<redacted>"
		} else if pattern == redactionPatterns[7] {
			replacement = "$1<path>"
		} else if pattern == redactionPatterns[4] || pattern == redactionPatterns[5] {
			replacement = "<ip>"
		} else if pattern == redactionPatterns[2] || pattern == redactionPatterns[3] {
			replacement = "<id>"
		} else if pattern == redactionPatterns[6] {
			replacement = "<path>"
		} else if pattern == redactionPatterns[8] {
			replacement = "$1<id>"
		}
		value = pattern.ReplaceAllString(value, replacement)
	}
	if len(value) > maxRedactedTextBytes {
		value = value[:maxRedactedTextBytes]
	}
	return value
}

func BuildAIInput(report crashreports.Report, metadata, stackwalk string) (AIInput, error) {
	payload := aiPayload{Metadata: Redact(metadata), Stackwalk: Redact(stackwalk)}
	if signature := report.ParsedSignature; signature != nil {
		payload.Platform = Redact(signature.Platform)
		payload.Architecture = Redact(signature.Architecture)
		payload.Crashed = Redact(signature.Crashed)
		payload.CrashReason = Redact(signature.CrashReason)
		payload.CrashAddress = Redact(signature.CrashAddress)
		payload.RequestingThread = signature.RequestingThread
		payload.Modules = make([]aiModule, 0, len(signature.Modules))
		for _, module := range signature.Modules {
			payload.Modules = append(payload.Modules, aiModule{
				DebugFile: Redact(module.DebugFile), DebugIdentifier: Redact(module.DebugIdentifier),
				CodeIdentifier: Redact(module.CodeIdentifier), Platform: Redact(module.Platform),
				Architecture: Redact(module.Architecture), Decision: Redact(module.Decision),
			})
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AIInput{}, err
	}
	digest := sha256.Sum256(body)
	return AIInput{Body: body, SHA256: hex.EncodeToString(digest[:])}, nil
}

func redactLines(value string) string {
	return strings.TrimSpace(Redact(value))
}
