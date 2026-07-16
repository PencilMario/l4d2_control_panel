package joblogs

import (
	"regexp"
	"sort"
	"strings"
)

var (
	authorizationPattern = regexp.MustCompile(`(?i)(authorization\s*:\s*)([^\s]+(?:\s+[^\s]+)?)`)
	cookiePattern        = regexp.MustCompile(`(?i)(cookie\s*:\s*)([^\r\n]+)`)
	sensitiveEnvPattern  = regexp.MustCompile(`(?i)([A-Z0-9_]*(?:PASSWORD|TOKEN|SECRET)[A-Z0-9_]*\s*=\s*)([^\s]+)`)
	ansiPattern          = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
)

type Redactor struct {
	secrets func() []string
}

func NewRedactor(secrets func() []string) Redactor {
	return Redactor{secrets: secrets}
}

func (r Redactor) Redact(value string) string {
	value = ansiPattern.ReplaceAllString(value, "")
	value = authorizationPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = cookiePattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = sensitiveEnvPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	if r.secrets == nil {
		return value
	}
	secrets := append([]string(nil), r.secrets()...)
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
