package crashreports

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxAIAnalysisBytes = 256 << 10

func (m *Manager) SetStackwalkStatus(ctx context.Context, id string, status AnalysisStatus, message string) error {
	if !validAnalysisStatus(status) {
		return errors.New("invalid stackwalk status")
	}
	return m.updateReportWithFile(ctx, id, "stackwalk.txt", false, nil, func(report *Report) error {
		report.StackwalkStatus = status
		report.StackwalkError = trimAnalysisError(message)
		if status != AnalysisStatusSucceeded {
			report.StackwalkTool = ""
		}
		return nil
	})
}

func (m *Manager) SaveStackwalk(ctx context.Context, id string, update StackwalkUpdate) error {
	if !validAnalysisStatus(update.Status) {
		return errors.New("invalid stackwalk status")
	}
	if len(update.Text) > maxAIAnalysisBytes*4 {
		return errors.New("stackwalk output exceeds persistence limit")
	}
	return m.updateReportWithFile(ctx, id, "stackwalk.txt", update.Status == AnalysisStatusSucceeded, []byte(update.Text), func(report *Report) error {
		report.StackwalkStatus = update.Status
		report.StackwalkError = trimAnalysisError(update.Error)
		report.StackwalkTool = ""
		if update.Status == AnalysisStatusSucceeded {
			report.StackwalkTool = strings.TrimSpace(update.Tool)
			report.StackwalkAt = m.currentTime()
		}
		return nil
	})
}

func (m *Manager) SetAIStatus(ctx context.Context, id string, update AIAnalysisUpdate) error {
	if !validAnalysisStatus(update.Status) {
		return errors.New("invalid AI analysis status")
	}
	return m.updateReportWithFile(ctx, id, "ai-analysis.md", false, nil, func(report *Report) error {
		report.AIStatus = update.Status
		report.AIError = trimAnalysisError(update.Error)
		report.AIModel = strings.TrimSpace(update.Model)
		report.AIInputSHA256 = update.InputSHA256
		if update.StartedAt.IsZero() && update.Status == AnalysisStatusRunning {
			update.StartedAt = m.currentTime()
		}
		if !update.StartedAt.IsZero() {
			report.AIStartedAt = update.StartedAt
		}
		if !update.CompletedAt.IsZero() {
			report.AICompletedAt = update.CompletedAt
		}
		if update.Status != AnalysisStatusSucceeded {
			report.AIAnalysis = ""
		}
		return nil
	})
}

func (m *Manager) SaveAIAnalysis(ctx context.Context, id string, update AIAnalysisUpdate) error {
	if !validAnalysisStatus(update.Status) {
		return errors.New("invalid AI analysis status")
	}
	if len(update.Text) > maxAIAnalysisBytes {
		return errors.New("AI analysis exceeds persistence limit")
	}
	return m.updateReportWithFile(ctx, id, "ai-analysis.md", update.Status == AnalysisStatusSucceeded, []byte(update.Text), func(report *Report) error {
		report.AIStatus = update.Status
		report.AIError = trimAnalysisError(update.Error)
		report.AIModel = strings.TrimSpace(update.Model)
		report.AIInputSHA256 = update.InputSHA256
		report.AIAnalysis = ""
		if update.Status == AnalysisStatusSucceeded {
			report.AIAnalysis = update.Text
		}
		if !update.StartedAt.IsZero() {
			report.AIStartedAt = update.StartedAt
		}
		if !update.CompletedAt.IsZero() {
			report.AICompletedAt = update.CompletedAt
		}
		return nil
	})
}

func (m *Manager) ReadMetadata(ctx context.Context, id string) ([]byte, Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, Report{}, err
	}
	if !identifierPattern.MatchString(id) {
		return nil, Report{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	report, err := readReport(filepath.Join(m.root, "reports", id))
	if err != nil {
		return nil, Report{}, err
	}
	metadataPath := filepath.Join(m.root, "reports", id, "metadata.txt")
	info, err := os.Lstat(metadataPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, Report{}, ErrNotFound
	}
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, Report{}, err
	}
	return data, report, nil
}

func (m *Manager) ReadStackwalk(ctx context.Context, id string) (string, Report, error) {
	if err := ctx.Err(); err != nil {
		return "", Report{}, err
	}
	if !identifierPattern.MatchString(id) {
		return "", Report{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	report, err := readReport(filepath.Join(m.root, "reports", id))
	if err != nil {
		return "", Report{}, err
	}
	stackwalkPath := filepath.Join(m.root, "reports", id, "stackwalk.txt")
	info, err := os.Lstat(stackwalkPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", Report{}, ErrNotFound
	}
	data, err := os.ReadFile(stackwalkPath)
	if err != nil {
		return "", Report{}, err
	}
	return string(data), report, nil
}

func (m *Manager) PrepareStackwalk(ctx context.Context, id string) (StackwalkInput, Report, func(), error) {
	if err := ctx.Err(); err != nil {
		return StackwalkInput{}, Report{}, func() {}, err
	}
	if !identifierPattern.MatchString(id) {
		return StackwalkInput{}, Report{}, func() {}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	report, err := readReport(filepath.Join(m.root, "reports", id))
	if err != nil {
		return StackwalkInput{}, Report{}, func() {}, err
	}
	sourcePath := filepath.Join(m.root, "reports", id, "minidump.dmp")
	info, err := os.Lstat(sourcePath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return StackwalkInput{}, Report{}, func() {}, ErrNotFound
	}
	temporaryDirectory, err := os.MkdirTemp(filepath.Join(m.root, "incoming"), ".analysis-")
	if err != nil {
		return StackwalkInput{}, Report{}, func() {}, err
	}
	cleanupOnce := sync.Once{}
	cleanup := func() { cleanupOnce.Do(func() { _ = os.RemoveAll(temporaryDirectory) }) }
	temporaryPath := filepath.Join(temporaryDirectory, "minidump.dmp")
	input, err := os.Open(sourcePath)
	if err != nil {
		cleanup()
		return StackwalkInput{}, Report{}, func() {}, err
	}
	output, err := os.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		input.Close()
		cleanup()
		return StackwalkInput{}, Report{}, func() {}, err
	}
	_, copyErr := io.Copy(output, input)
	closeInputErr := input.Close()
	closeOutputErr := output.Close()
	if joined := errors.Join(copyErr, closeInputErr, closeOutputErr); joined != nil {
		cleanup()
		return StackwalkInput{}, Report{}, func() {}, joined
	}
	return StackwalkInput{DumpPath: temporaryPath, SymbolRoot: filepath.Join(m.root, "symbols")}, report, cleanup, nil
}

func (m *Manager) updateReport(ctx context.Context, id string, update func(*Report) error) error {
	return m.updateReportWithFile(ctx, id, "", false, nil, update)
}

func (m *Manager) updateReportWithFile(ctx context.Context, id, fileName string, keepFile bool, content []byte, update func(*Report) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !identifierPattern.MatchString(id) {
		return ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	reportDir := filepath.Join(m.root, "reports", id)
	report, err := readReport(reportDir)
	if err != nil {
		return err
	}
	if err := update(&report); err != nil {
		return err
	}
	if fileName != "" {
		path := filepath.Join(reportDir, fileName)
		if keepFile {
			if err := writeBytesAtomic(path, content, 0o600); err != nil {
				return err
			}
		} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	report.UpdatedAt = m.currentTime()
	return writeJSONAtomic(filepath.Join(reportDir, "report.json"), report, 0o600)
}

func validAnalysisStatus(status AnalysisStatus) bool {
	switch status {
	case AnalysisStatusQueued, AnalysisStatusRunning, AnalysisStatusSucceeded, AnalysisStatusFailed, AnalysisStatusUnconfigured:
		return true
	default:
		return false
	}
}

func trimAnalysisError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1024 {
		value = value[:1024]
	}
	return value
}

func (m *Manager) RecoverAnalysis(ctx context.Context) ([]AnalysisRecovery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reports, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	queued := make([]AnalysisRecovery, 0)
	for _, report := range reports {
		if report.StackwalkStatus == AnalysisStatusRunning {
			if err := m.SetStackwalkStatus(ctx, report.ID, AnalysisStatusQueued, "recovered after worker restart"); err != nil {
				return nil, err
			}
		}
		requestAI := report.AIStatus == AnalysisStatusQueued || report.AIStatus == AnalysisStatusRunning
		if report.AIStatus == AnalysisStatusRunning {
			if err := m.SetAIStatus(ctx, report.ID, AIAnalysisUpdate{Status: AnalysisStatusQueued, Model: report.AIModel, Error: "recovered after worker restart"}); err != nil {
				return nil, err
			}
		}
		if report.StackwalkStatus == AnalysisStatusQueued || report.StackwalkStatus == AnalysisStatusRunning || requestAI {
			queued = append(queued, AnalysisRecovery{ID: report.ID, RequestAI: requestAI})
		}
	}
	return queued, nil
}
