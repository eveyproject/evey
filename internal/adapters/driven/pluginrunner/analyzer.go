package pluginrunner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

type AnalysisSeverity int

const (
	SeverityInfo     AnalysisSeverity = 1
	SeverityWarning  AnalysisSeverity = 2
	SeverityCritical AnalysisSeverity = 3
)

type AnalysisFinding struct {
	Severity    AnalysisSeverity
	Category    string
	File        string
	Line        int
	Pattern     string
	Description string
}

type AnalysisReport struct {
	Findings          []AnalysisFinding
	TotalFilesScanned int
	Safe              bool
}

type analyzerRule struct {
	pattern     *regexp.Regexp
	severity    AnalysisSeverity
	category    string
	description string
	extensions  []string // empty = all
}

var analyzerRules = []analyzerRule{
	// Shell execution
	{regexp.MustCompile(`(?i)(os\.system|subprocess\.(call|run|Popen)|exec\(|child_process\.exec|spawn\()`), SeverityCritical, "shell_exec", "shell command execution", nil},
	{regexp.MustCompile(`(?i)(os\.popen|commands\.getoutput|backtick)`), SeverityCritical, "shell_exec", "shell command execution via pipe", nil},

	// Network — outbound connections
	{regexp.MustCompile(`(?i)(socket\.connect|http\.request|urllib\.request|requests\.(get|post|put|delete)|fetch\(|axios\.|net\.Dial)`), SeverityWarning, "network", "outbound network connection", nil},
	{regexp.MustCompile(`(?i)(reverse.?shell|bind.?shell|nc\s+-[elp]|ncat\s|socat\s)`), SeverityCritical, "network", "potential reverse/bind shell", nil},

	// Filesystem — dangerous access
	{regexp.MustCompile(`(?i)(\/etc\/passwd|\/etc\/shadow|\.ssh\/|authorized_keys|id_rsa)`), SeverityCritical, "filesystem", "access to sensitive system files", nil},
	{regexp.MustCompile(`(?i)(shutil\.rmtree|rimraf|rm\s+-rf|os\.remove|unlink\()`), SeverityWarning, "filesystem", "recursive file deletion", nil},
	{regexp.MustCompile(`(?i)(symlink|os\.link|os\.symlink|fs\.symlinkSync)`), SeverityWarning, "filesystem", "symlink creation", nil},

	// Code obfuscation
	{regexp.MustCompile(`(?i)(eval\(|exec\(|compile\(.*exec)`), SeverityCritical, "obfuscation", "dynamic code execution (eval/exec)", nil},
	{regexp.MustCompile(`(?i)(base64\.b64decode|atob\(|Buffer\.from\(.*base64)`), SeverityWarning, "obfuscation", "base64 decoding (potential obfuscated payload)", nil},
	{regexp.MustCompile(`\\x[0-9a-fA-F]{2}.*\\x[0-9a-fA-F]{2}.*\\x[0-9a-fA-F]{2}`), SeverityWarning, "obfuscation", "hex-encoded string sequence", nil},

	// Privilege escalation
	{regexp.MustCompile(`(?i)(setuid|setgid|chmod\s+[47]777|chmod\s+u\+s)`), SeverityCritical, "privilege", "privilege escalation attempt", nil},
	{regexp.MustCompile(`(?i)(sudo\s|su\s+-|doas\s)`), SeverityCritical, "privilege", "sudo/su usage", nil},

	// Crypto mining
	{regexp.MustCompile(`(?i)(stratum\+tcp|xmrig|coinhive|cryptonight|monero)`), SeverityCritical, "cryptomining", "crypto mining indicator", nil},

	// Environment variable exfiltration
	{regexp.MustCompile(`(?i)(os\.environ|process\.env|ENV\[|getenv\()`), SeverityWarning, "env_access", "environment variable access", nil},

	// Package install at runtime
	{regexp.MustCompile(`(?i)(pip\s+install|npm\s+install|yarn\s+add|go\s+install|curl\s.*\|\s*(sh|bash))`), SeverityCritical, "runtime_install", "runtime package installation", nil},
}

var analyzedExtensions = map[string]bool{
	".py": true, ".js": true, ".ts": true, ".go": true,
	".sh": true, ".bash": true, ".rb": true, ".pl": true,
	".lua": true, ".php": true, ".yaml": true, ".yml": true,
	".json": true, ".toml": true, ".cfg": true, ".ini": true,
}

// StaticAnalyzer implements the domain PluginAnalyzer interface.
type StaticAnalyzer struct{}

func NewStaticAnalyzer() *StaticAnalyzer { return &StaticAnalyzer{} }

func (a *StaticAnalyzer) Analyze(dir string) (*plugin.AnalysisReport, error) {
	raw, err := AnalyzePlugin(dir)
	if err != nil {
		return nil, err
	}
	report := &plugin.AnalysisReport{
		Safe:      raw.Safe,
		FileCount: raw.TotalFilesScanned,
	}
	for _, f := range raw.Findings {
		report.Issues = append(report.Issues, fmt.Sprintf("[%s] %s:%d — %s (%s)", severityLabel(f.Severity), f.File, f.Line, f.Description, f.Pattern))
	}
	return report, nil
}

func severityLabel(s AnalysisSeverity) string {
	switch s {
	case SeverityCritical:
		return "CRITICAL"
	case SeverityWarning:
		return "WARNING"
	default:
		return "INFO"
	}
}

// AnalyzePlugin scans a plugin directory for suspicious patterns.
func AnalyzePlugin(dir string) (*AnalysisReport, error) {
	report := &AnalysisReport{Safe: true}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "node_modules" || base == ".git" || base == "__pycache__" || base == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !analyzedExtensions[ext] {
			return nil
		}

		report.TotalFilesScanned++
		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			relPath = path
		}

		findings, scanErr := scanFile(path, relPath)
		if scanErr != nil {
			return scanErr
		}
		report.Findings = append(report.Findings, findings...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk plugin dir: %w", err)
	}

	for _, f := range report.Findings {
		if f.Severity == SeverityCritical {
			report.Safe = false
			break
		}
	}

	return report, nil
}

func scanFile(path, relPath string) ([]AnalysisFinding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []AnalysisFinding
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		for _, rule := range analyzerRules {
			if len(rule.extensions) > 0 {
				ext := strings.ToLower(filepath.Ext(path))
				if !slices.Contains(rule.extensions, ext) {
					continue
				}
			}

			if loc := rule.pattern.FindString(line); loc != "" {
				findings = append(findings, AnalysisFinding{
					Severity:    rule.severity,
					Category:    rule.category,
					File:        relPath,
					Line:        lineNum,
					Pattern:     loc,
					Description: rule.description,
				})
			}
		}
	}

	return findings, scanner.Err()
}
