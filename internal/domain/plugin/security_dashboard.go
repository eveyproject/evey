package plugin

import "time"

// SecurityFinding is a single static-analysis finding stored per plugin.
type SecurityFinding struct {
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Description string `json:"description,omitempty"`
}

// PluginLastError holds the most recent plugin.error event for a plugin.
type PluginLastError struct {
	Ts      time.Time `json:"ts"`
	Message string    `json:"message"`
}

// SignatureStatus describes the cosign verification result for a plugin.
type SignatureStatus struct {
	Verified  bool   `json:"verified"`
	VerifiedBy string `json:"verifiedBy,omitempty"` // key identifier / issuer
	Error     string `json:"error,omitempty"`
}

// PluginSecuritySnapshot aggregates all security-relevant data for one plugin.
type PluginSecuritySnapshot struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Version       string           `json:"version"`
	SandboxMode   string           `json:"sandboxMode"`
	Permissions   []string         `json:"permissions"`
	Findings      []SecurityFinding `json:"findings"`
	LastError     *PluginLastError  `json:"lastError,omitempty"`
	Signature     *SignatureStatus  `json:"signature,omitempty"`
	SourceSHA256  string            `json:"sourceSHA256"`
	InstalledAt   time.Time         `json:"installedAt"`
	LastStartedAt *time.Time        `json:"lastStartedAt,omitempty"`
	SizeBytes     int64             `json:"sizeBytes"`
}
