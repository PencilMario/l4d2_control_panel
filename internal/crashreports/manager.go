package crashreports

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var identifierPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Manager struct {
	root                 string
	token                string
	retention            time.Duration
	now                  func() time.Time
	authorizeInstance    InstanceAuthorizer
	resolveInstance      InstanceResolver
	enqueueAnalysis      func(context.Context, Report) error
	analysisEnqueueError func(error)
	reportCleanupError   func(error)
	mu                   sync.Mutex
}

func New(config Config) (*Manager, error) {
	if strings.TrimSpace(config.Root) == "" {
		return nil, fmt.Errorf("%w: root is required", ErrInvalidConfig)
	}
	retentionDays := config.RetentionDays
	if retentionDays == 0 {
		retentionDays = DefaultRetentionDays
	}
	if retentionDays < MinRetentionDays || retentionDays > MaxRetentionDays {
		return nil, fmt.Errorf("%w: retention days must be between %d and %d", ErrInvalidConfig, MinRetentionDays, MaxRetentionDays)
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	root, err := filepath.Abs(filepath.Clean(config.Root))
	if err != nil {
		return nil, fmt.Errorf("resolve crash report root: %w", err)
	}
	for _, name := range []string{"incoming", "pending", "reports", "symbols", filepath.Join("symbols", "uploaded"), filepath.Join("symbols", "builtin"), "binaries", "symbol-manifests", "binary-manifests"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o750); err != nil {
			return nil, fmt.Errorf("create crash report directory %s: %w", name, err)
		}
	}
	manager := &Manager{root: root, token: config.Token, retention: time.Duration(retentionDays) * 24 * time.Hour, now: now, authorizeInstance: config.AuthorizeInstance, resolveInstance: config.ResolveInstance, enqueueAnalysis: config.EnqueueAnalysis, analysisEnqueueError: config.AnalysisEnqueueError, reportCleanupError: config.ReportCleanupError}
	if err := manager.installBuiltinSymbols(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) Authorized(token string) bool {
	if m.token == "" {
		return false
	}
	return hmac.Equal([]byte(m.token), []byte(token))
}

func (m *Manager) Configured() bool { return m.token != "" }

func (m *Manager) PreSubmit(input PreSubmitInput) (string, error) {
	if len(input.CrashSignature) > MaxCrashSignatureBytes {
		return "", ErrCrashSignatureTooLarge
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	decisions, err := m.moduleDecisions(input.CrashSignature)
	if err != nil {
		return "", err
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	pending := PendingSubmission{Token: token, CreatedAt: m.currentTime(), Input: input}
	if err := writeJSONAtomic(filepath.Join(m.root, "pending", token+".json"), pending, 0o600); err != nil {
		return "", fmt.Errorf("save pending crash report: %w", err)
	}
	return "Y|" + decisions + "|" + token, nil
}

func (m *Manager) moduleDecisions(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	signature, err := ParseCrashSignature(raw)
	if err != nil {
		// Preserve the receiver's legacy compatibility for pre-v2 senders.
		if strings.HasPrefix(raw, "2|") {
			return "", err
		}
		count := countModules(raw)
		if count > MaxModuleCount {
			return "", ErrTooManyModules
		}
		return strings.Repeat("N", count), nil
	}
	decisions := make([]byte, len(signature.Modules))
	for index, module := range signature.Modules {
		decisions[index] = 'N'
		if module.DebugIdentifier == "" {
			continue
		}
		if _, found := m.hasMatchingArtifact(module, ArtifactKindSymbol); found {
			continue
		}
		if strings.Contains(signature.Platform, "linux") {
			decisions[index] = 'Y'
			continue
		}
		if strings.Contains(signature.Platform, "windows") {
			if _, found := m.hasMatchingArtifact(module, ArtifactKindBinary); !found {
				decisions[index] = 'U'
			}
		}
	}
	return string(decisions), nil
}

func countModules(signature string) int {
	parts := strings.Split(signature, "|")
	count := 0
	for _, part := range parts {
		if part == "M" {
			count++
		}
	}
	return count
}

func (m *Manager) Receive(ctx context.Context, input UploadInput) (report Report, err error) {
	if input.Minidump == nil {
		return Report{}, ErrInvalidMinidump
	}
	if input.Metadata == nil {
		return Report{}, ErrMetadataRequired
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.validatePendingBinding(input.PresubmitToken, input.InstanceID, input.ServerID); err != nil {
		return Report{}, err
	}

	dumpPath, dumpSize, digest, err := m.writeIncoming(ctx, "minidump", input.Minidump, MaxMinidumpBytes, ErrMinidumpTooLarge)
	if err != nil {
		return Report{}, err
	}
	defer os.Remove(dumpPath)
	if err := hasMinidumpHeader(dumpPath); err != nil {
		return Report{}, err
	}
	metadataPath, metadataSize, _, err := m.writeIncoming(ctx, "metadata", input.Metadata, MaxMetadataBytes, ErrMetadataTooLarge)
	if err != nil {
		return Report{}, err
	}
	defer os.Remove(metadataPath)

	id := hex.EncodeToString(digest[:])
	reportDir := filepath.Join(m.root, "reports", id)
	if !identifierPattern.MatchString(id) {
		return Report{}, ErrNotFound
	}
	createdReportDir := false
	if info, statErr := os.Lstat(reportDir); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return Report{}, fmt.Errorf("report path is not a directory")
		}
	} else if !os.IsNotExist(statErr) {
		return Report{}, statErr
	} else if err := os.Mkdir(reportDir, 0o750); err != nil {
		return Report{}, fmt.Errorf("create report directory: %w", err)
	} else {
		createdReportDir = true
	}

	old, oldErr := readReport(reportDir)
	if oldErr != nil && !errors.Is(oldErr, ErrNotFound) {
		return Report{}, oldErr
	}
	var oldMetadata []byte
	oldMetadataAvailable := false
	metadataWasPresent := false
	metadataWasRegular := false
	metadataTarget := filepath.Join(reportDir, "metadata.txt")
	var oldManifest []byte
	oldManifestAvailable := false
	metadataCommitAttempted := false
	manifestCommitAttempted := false
	minidumpInstalled := false
	defer func() {
		if err == nil {
			return
		}
		if minidumpInstalled {
			_ = os.Remove(filepath.Join(reportDir, "minidump.dmp"))
		}
		if metadataCommitAttempted {
			switch {
			case oldMetadataAvailable:
				_ = writeBytesAtomic(metadataTarget, oldMetadata, 0o600)
			case !metadataWasPresent:
				_ = os.Remove(metadataTarget)
			}
		}
		if manifestCommitAttempted {
			switch {
			case oldManifestAvailable:
				_ = writeBytesAtomic(filepath.Join(reportDir, "report.json"), oldManifest, 0o600)
			case oldErr != nil:
				_ = os.Remove(filepath.Join(reportDir, "report.json"))
			}
		}
		if createdReportDir {
			_ = os.RemoveAll(reportDir)
		}
	}()

	oldMetadata, metadataWasPresent, metadataWasRegular, err = snapshotFile(metadataTarget)
	if err != nil {
		return Report{}, err
	}
	oldMetadataAvailable = metadataWasRegular
	if oldErr == nil {
		oldManifest, err = os.ReadFile(filepath.Join(reportDir, "report.json"))
		if err != nil {
			return Report{}, err
		}
		oldManifestAvailable = true
	}

	report = Report{
		ID:               id,
		InstanceID:       input.InstanceID,
		ReceivedAt:       m.currentTime(),
		UpdatedAt:        m.currentTime(),
		MinidumpSize:     dumpSize,
		MetadataSize:     metadataSize,
		SHA256:           id,
		UserID:           input.UserID,
		GameDirectory:    input.GameDirectory,
		ExtensionVersion: input.ExtensionVersion,
		ServerID:         input.ServerID,
		PresubmitToken:   input.PresubmitToken,
	}
	if oldErr == nil {
		report.ReceivedAt = old.ReceivedAt
		if report.UserID == "" {
			report.UserID = old.UserID
		}
		if report.InstanceID == "" {
			report.InstanceID = old.InstanceID
		}
		if report.GameDirectory == "" {
			report.GameDirectory = old.GameDirectory
		}
		if report.ExtensionVersion == "" {
			report.ExtensionVersion = old.ExtensionVersion
		}
		if report.ServerID == "" {
			report.ServerID = old.ServerID
		}
		if report.ParsedSignature == nil {
			report.ParsedSignature = old.ParsedSignature
		}
		if len(report.Modules) == 0 {
			report.Modules = append([]Module(nil), old.Modules...)
		}
		if report.CrashSignature == "" {
			report.CrashSignature = old.CrashSignature
		}
		report.StackwalkStatus = old.StackwalkStatus
		report.StackwalkError = old.StackwalkError
		report.StackwalkTool = old.StackwalkTool
		report.StackwalkAt = old.StackwalkAt
		report.AIStatus = old.AIStatus
		report.AIError = old.AIError
		report.AIModel = old.AIModel
		report.AIInputSHA256 = old.AIInputSHA256
		report.AIAnalysis = old.AIAnalysis
		report.AIStartedAt = old.AIStartedAt
		report.AICompletedAt = old.AICompletedAt
	}
	if pending, pendingErr := m.readPending(input.PresubmitToken); pendingErr == nil {
		report.CrashSignature = pending.Input.CrashSignature
		if report.InstanceID == "" {
			report.InstanceID = pending.Input.InstanceID
		}
		if report.UserID == "" {
			report.UserID = pending.Input.UserID
		}
		if report.ExtensionVersion == "" {
			report.ExtensionVersion = pending.Input.ExtensionVersion
		}
		if report.ServerID == "" {
			report.ServerID = pending.Input.ServerID
		}
	}
	if report.CrashSignature != "" {
		if parsed, parseErr := ParseCrashSignature(report.CrashSignature); parseErr == nil {
			report.ParsedSignature = &parsed
			report.Modules = append([]Module(nil), parsed.Modules...)
			for index := range report.Modules {
				if symbol, found := m.hasMatchingArtifact(report.Modules[index], ArtifactKindSymbol); found {
					report.Modules[index].SymbolArtifact = symbol.ID
				}
				if binary, found := m.hasMatchingArtifact(report.Modules[index], ArtifactKindBinary); found {
					report.Modules[index].BinaryArtifact = binary.ID
				}
			}
		}
	}
	if oldErr != nil {
		minidumpPath := filepath.Join(reportDir, "minidump.dmp")
		if err := os.Rename(dumpPath, minidumpPath); err != nil {
			return Report{}, fmt.Errorf("move minidump: %w", err)
		}
		minidumpInstalled = true
		if err := os.Chmod(minidumpPath, 0o600); err != nil {
			return Report{}, fmt.Errorf("protect minidump: %w", err)
		}
	} else if err := os.Remove(dumpPath); err != nil && !os.IsNotExist(err) {
		return Report{}, fmt.Errorf("discard duplicate minidump: %w", err)
	}
	metadataCommitAttempted = true
	if err := replaceFile(metadataPath, metadataTarget); err != nil {
		return Report{}, fmt.Errorf("move metadata: %w", err)
	}
	if err := os.Chmod(metadataTarget, 0o600); err != nil {
		return Report{}, fmt.Errorf("protect metadata: %w", err)
	}
	manifestCommitAttempted = true
	if err := writeJSONAtomic(filepath.Join(reportDir, "report.json"), report, 0o600); err != nil {
		return Report{}, fmt.Errorf("write report manifest: %w", err)
	}
	if tokenPattern.MatchString(input.PresubmitToken) {
		_ = os.Remove(m.pendingPath(input.PresubmitToken))
	}
	if oldErr != nil && m.enqueueAnalysis != nil {
		queued := report
		go func() {
			if enqueueErr := m.enqueueAnalysis(context.Background(), queued); enqueueErr != nil && m.analysisEnqueueError != nil {
				m.analysisEnqueueError(enqueueErr)
			}
		}()
	}
	return report, nil
}

func (m *Manager) writeIncoming(ctx context.Context, label string, source io.Reader, limit int64, tooLarge error) (path string, total int64, checksum [32]byte, err error) {
	temporary, err := os.CreateTemp(filepath.Join(m.root, "incoming"), "."+label+"-*")
	if err != nil {
		return "", 0, [32]byte{}, err
	}
	path = temporary.Name()
	removeOnReturn := true
	defer func() {
		_ = temporary.Close()
		if removeOnReturn {
			_ = os.Remove(path)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return path, 0, [32]byte{}, err
	}
	digest := sha256.New()
	writer := io.MultiWriter(temporary, digest)
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return path, total, [32]byte{}, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			if total+int64(read) > limit {
				return path, total, [32]byte{}, tooLarge
			}
			if _, err := writer.Write(buffer[:read]); err != nil {
				return path, total, [32]byte{}, err
			}
			total += int64(read)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return path, total, [32]byte{}, readErr
		}
	}
	if err := temporary.Sync(); err != nil {
		return path, total, [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], digest.Sum(nil))
	removeOnReturn = false
	return path, total, result, nil
}

func hasMinidumpHeader(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil || string(header) != "MDMP" {
		return ErrInvalidMinidump
	}
	return nil
}

func (m *Manager) SaveSymbol(ctx context.Context, input SymbolInput) error {
	if input.Symbol == nil || !validOptionalIdentifier(input.DebugIdentifier) || !validOptionalIdentifier(input.CodeIdentifier) {
		return ErrInvalidSymbol
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.validatePendingBinding(input.PresubmitToken, input.InstanceID, input.ServerID); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Join(m.root, "incoming"), ".symbol-*")
	if err != nil {
		return err
	}
	path := temporary.Name()
	defer func() {
		temporary.Close()
		os.Remove(path)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	digest := sha256.New()
	var total int64
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, readErr := input.Symbol.Read(buffer)
		if read > 0 {
			if total+int64(read) > MaxSymbolBytes {
				return ErrSymbolTooLarge
			}
			if _, err := temporary.Write(buffer[:read]); err != nil {
				return err
			}
			if _, err := digest.Write(buffer[:read]); err != nil {
				return err
			}
			total += int64(read)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return readErr
		}
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		return ErrInvalidSymbol
	}
	parsedPlatform, parsedArchitecture, parsedDebugIdentifier, parsedDebugFile := parseBreakpadModule(raw)
	if input.DebugIdentifier == "" {
		input.DebugIdentifier = parsedDebugIdentifier
	}
	if input.DebugIdentifier == "" || !validIdentifier(input.DebugIdentifier) {
		return ErrInvalidSymbol
	}
	if parsedDebugIdentifier != "" {
		// Breakpad's MODULE record is the authoritative identity. The upstream
		// Accelerator symbol endpoint does not send a separate identifier field.
		input.DebugIdentifier = parsedDebugIdentifier
	}
	if input.Platform == "" {
		input.Platform = parsedPlatform
	}
	if input.Architecture == "" {
		input.Architecture = parsedArchitecture
	}
	if input.Platform != "" {
		input.Platform = strings.ToLower(input.Platform)
	}
	if input.Architecture != "" {
		input.Architecture = strings.ToLower(input.Architecture)
	}
	if input.DebugFile == "" {
		input.DebugFile = parsedDebugFile
	}
	name := hex.EncodeToString(digest.Sum(nil)) + ".sym"
	id := strings.TrimSuffix(name, ".sym")
	target := m.artifactPath(ArtifactKindSymbol, id, false)
	installed := false
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrInvalidSymbol
		}
	} else if os.IsNotExist(err) {
		if err := os.Rename(path, target); err != nil {
			return err
		}
		installed = true
	} else {
		return err
	}
	artifact := Artifact{
		ID: id, Kind: ArtifactKindSymbol, SHA256: id, Size: total,
		InstanceID: input.InstanceID, DebugIdentifier: input.DebugIdentifier,
		CodeIdentifier: input.CodeIdentifier, DebugFile: input.DebugFile,
		Platform: input.Platform, Architecture: input.Architecture,
		ReceivedAt: m.currentTime().Format(timeFormat),
	}
	if err := m.ensureSymbolLookup(artifact, target); err != nil {
		if installed {
			_ = os.Remove(target)
		}
		return err
	}
	if err := m.writeArtifactManifest(artifact); err != nil {
		if installed {
			_ = os.Remove(target)
		}
		return err
	}
	if err := m.backfillArtifactReferences(ctx, artifact); err != nil {
		return err
	}
	return nil
}

func (m *Manager) List(ctx context.Context) ([]Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked(ctx)
}

func (m *Manager) listLocked(ctx context.Context) ([]Report, error) {
	entries, err := os.ReadDir(filepath.Join(m.root, "reports"))
	if err != nil {
		return nil, err
	}
	reports := make([]Report, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !identifierPattern.MatchString(entry.Name()) {
			continue
		}
		info, err := os.Lstat(filepath.Join(m.root, "reports", entry.Name()))
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		report, err := readReport(filepath.Join(m.root, "reports", entry.Name()))
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	sort.SliceStable(reports, func(i, j int) bool {
		return reports[i].ReceivedAt.After(reports[j].ReceivedAt)
	})
	return reports, nil
}

func (m *Manager) Get(ctx context.Context, id string) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if !identifierPattern.MatchString(id) {
		return Report{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return readReport(filepath.Join(m.root, "reports", id))
}

func (m *Manager) Open(ctx context.Context, id string, kind FileKind) (*os.File, Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, Report{}, err
	}
	name := ""
	switch kind {
	case FileKindMinidump:
		name = "minidump.dmp"
	case FileKindMetadata:
		name = "metadata.txt"
	case FileKindStackwalk:
		name = "stackwalk.txt"
	case FileKindAI:
		name = "ai-analysis.md"
	default:
		return nil, Report{}, ErrInvalidFileKind
	}
	if !identifierPattern.MatchString(id) {
		return nil, Report{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	reportDir := filepath.Join(m.root, "reports", id)
	report, err := readReport(reportDir)
	if err != nil {
		return nil, Report{}, err
	}
	path := filepath.Join(reportDir, name)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, Report{}, ErrNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, Report{}, err
	}
	return file, report, nil
}

func (m *Manager) Cleanup(ctx context.Context) (CleanupResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return CleanupResult{}, err
	}
	var result CleanupResult
	cutoff := m.currentTime().Add(-m.retention)
	entries, err := os.ReadDir(filepath.Join(m.root, "reports"))
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if !identifierPattern.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(m.root, "reports", entry.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		report, err := readReport(path)
		if err != nil || !report.ReceivedAt.Before(cutoff) {
			continue
		}
		if size, sizeErr := directorySize(path); sizeErr == nil {
			result.BytesReleased += size
		}
		if err := os.RemoveAll(path); err != nil {
			return result, err
		}
		result.ReportsRemoved++
	}
	pendingEntries, err := os.ReadDir(filepath.Join(m.root, "pending"))
	if err != nil {
		return result, err
	}
	pendingCutoff := m.currentTime().Add(-PendingRetention)
	for _, entry := range pendingEntries {
		if !strings.HasSuffix(entry.Name(), ".json") || !tokenPattern.MatchString(strings.TrimSuffix(entry.Name(), ".json")) {
			continue
		}
		path := filepath.Join(m.root, "pending", entry.Name())
		var pending PendingSubmission
		raw, readErr := os.ReadFile(path)
		if readErr != nil || json.Unmarshal(raw, &pending) != nil || pending.CreatedAt.After(pendingCutoff) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return result, err
		}
		result.PendingRemoved++
	}
	artifactResult, err := m.cleanupArtifacts(ctx, cutoff)
	if err != nil {
		return result, err
	}
	result.ArtifactsRemoved = artifactResult.ArtifactsRemoved
	result.BytesReleased += artifactResult.BytesReleased
	return result, nil
}

func (m *Manager) cleanupArtifacts(ctx context.Context, cutoff time.Time) (CleanupResult, error) {
	var result CleanupResult
	referenced := make(map[string]struct{})
	reportEntries, err := os.ReadDir(filepath.Join(m.root, "reports"))
	if err != nil {
		return result, err
	}
	for _, entry := range reportEntries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !identifierPattern.MatchString(entry.Name()) {
			continue
		}
		reportPath := filepath.Join(m.root, "reports", entry.Name())
		info, statErr := os.Lstat(reportPath)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		report, readErr := readReport(reportPath)
		if errors.Is(readErr, ErrNotFound) {
			continue
		}
		if readErr != nil {
			return result, readErr
		}
		for _, module := range report.Modules {
			if module.SymbolArtifact != "" {
				referenced[artifactReferenceKey(ArtifactKindSymbol, module.SymbolArtifact)] = struct{}{}
			}
			if module.BinaryArtifact != "" {
				referenced[artifactReferenceKey(ArtifactKindBinary, module.BinaryArtifact)] = struct{}{}
			}
		}
	}

	for _, item := range []struct {
		kind      ArtifactKind
		directory string
	}{
		{kind: ArtifactKindSymbol, directory: "symbol-manifests"},
		{kind: ArtifactKindBinary, directory: "binary-manifests"},
	} {
		entries, err := os.ReadDir(filepath.Join(m.root, item.directory))
		if err != nil {
			return result, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			if !strings.HasSuffix(entry.Name(), ".json") || !identifierPattern.MatchString(id) {
				continue
			}
			manifestPath := filepath.Join(m.root, item.directory, entry.Name())
			manifestInfo, statErr := os.Lstat(manifestPath)
			if statErr != nil || manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
				continue
			}
			raw, readErr := os.ReadFile(manifestPath)
			if readErr != nil {
				return result, readErr
			}
			var artifact Artifact
			if err := json.Unmarshal(raw, &artifact); err != nil || artifact.ID != id || artifact.Kind != item.kind || artifact.SHA256 != id {
				continue
			}
			if artifact.Builtin || !artifactTimeBefore(artifact, cutoff) || hasArtifactReference(referenced, item.kind, id) {
				continue
			}
			objectPath := m.artifactPath(item.kind, id, false)
			objectInfo, objectErr := os.Lstat(objectPath)
			if objectErr != nil && !os.IsNotExist(objectErr) {
				return result, objectErr
			}
			if objectErr == nil && (objectInfo.Mode()&os.ModeSymlink != 0 || !objectInfo.Mode().IsRegular()) {
				continue
			}
			if objectErr == nil {
				result.BytesReleased += objectInfo.Size()
			}
			if item.kind == ArtifactKindSymbol {
				lookupSize, lookupErr := m.removeSymbolLookup(artifact)
				if lookupErr != nil {
					return result, lookupErr
				}
				result.BytesReleased += lookupSize
			}
			result.BytesReleased += manifestInfo.Size()
			if objectErr == nil {
				if err := os.Remove(objectPath); err != nil {
					return result, err
				}
			}
			if err := os.Remove(manifestPath); err != nil {
				return result, err
			}
			result.ArtifactsRemoved++
		}
	}
	return result, nil
}

func artifactReferenceKey(kind ArtifactKind, id string) string {
	return string(kind) + ":" + id
}

func hasArtifactReference(referenced map[string]struct{}, kind ArtifactKind, id string) bool {
	_, ok := referenced[artifactReferenceKey(kind, id)]
	return ok
}

func artifactTimeBefore(artifact Artifact, cutoff time.Time) bool {
	receivedAt, err := time.Parse(timeFormat, artifact.ReceivedAt)
	return err == nil && receivedAt.Before(cutoff)
}

func (m *Manager) StartCleanup(parent context.Context) func() {
	return m.startCleanup(parent, 24*time.Hour)
}

func (m *Manager) startCleanup(parent context.Context, interval time.Duration) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := m.Cleanup(ctx); err != nil && !errors.Is(err, context.Canceled) {
					if m.reportCleanupError != nil {
						m.reportCleanupError(err)
					}
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func (m *Manager) readPending(token string) (PendingSubmission, error) {
	if !tokenPattern.MatchString(token) {
		return PendingSubmission{}, ErrNotFound
	}
	raw, err := os.ReadFile(m.pendingPath(token))
	if err != nil {
		return PendingSubmission{}, err
	}
	var pending PendingSubmission
	if err := json.Unmarshal(raw, &pending); err != nil {
		return PendingSubmission{}, err
	}
	if !pending.CreatedAt.Add(PendingRetention).After(m.currentTime()) {
		return PendingSubmission{}, ErrNotFound
	}
	return pending, nil
}

func (m *Manager) validatePendingBinding(token, instanceID, serverID string) error {
	pending, err := m.readPending(token)
	if err != nil {
		return nil
	}
	if pending.Input.InstanceID != "" && instanceID != "" && pending.Input.InstanceID != instanceID {
		return ErrInstanceNotAllowed
	}
	if pending.Input.ServerID != "" && serverID != "" && pending.Input.ServerID != serverID {
		return ErrInstanceNotAllowed
	}
	return nil
}

func (m *Manager) pendingPath(token string) string {
	return filepath.Join(m.root, "pending", token+".json")
}

func (m *Manager) currentTime() time.Time {
	return m.now().UTC()
}

func readReport(dir string) (Report, error) {
	info, err := os.Lstat(dir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Report{}, ErrNotFound
	}
	raw, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return Report{}, ErrNotFound
		}
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return Report{}, err
	}
	if !identifierPattern.MatchString(report.ID) || report.ID != filepath.Base(dir) {
		return Report{}, ErrNotFound
	}
	return report, nil
}

func snapshotFile(path string) (data []byte, exists, regular bool, err error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, true, false, nil
	}
	data, err = os.ReadFile(path)
	return data, true, err == nil, err
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	return writeBytesAtomic(path, buffer.Bytes(), mode)
}

func writeBytesAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".atomic-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() {
		file.Close()
		os.Remove(temporary)
	}()
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}

func replaceFile(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	} else if _, statErr := os.Lstat(target); statErr != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		return err
	}
	return os.Rename(source, target)
}

func randomToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}
