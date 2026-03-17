package pluginrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

var dangerousPatterns = []string{
	"os/exec",
	"syscall",
	"unsafe",
	"net/http",
	"crypto/tls",
}

type StaticAnalyzer struct{}

func NewStaticAnalyzer() *StaticAnalyzer { return &StaticAnalyzer{} }

func (a *StaticAnalyzer) Analyze(dir string) (*plugin.AnalysisReport, error) {
	report := &plugin.AnalysisReport{Safe: true}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		report.FileCount++

		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".py" && ext != ".js" && ext != ".ts" && ext != ".jsx" && ext != ".tsx" {
			return nil
		}

		data, readErr := os.ReadFile(path) //nolint:gosec
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		content := string(data)

		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			relPath = path
		}
		for _, pattern := range dangerousPatterns {
			if strings.Contains(content, pattern) {
				report.Issues = append(report.Issues, fmt.Sprintf("%s: contains %q", relPath, pattern))
				report.Safe = false
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk plugin dir: %w", err)
	}
	return report, nil
}
