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

// SignatureStatus describes the cosign verification result for a plugin. The
// signing key travels with the package (cosign.pub at the ZIP root); the
// fingerprint identifies it, the instance's trusted-publisher list names it.
type SignatureStatus struct {
	Verified   bool   `json:"verified"`
	VerifiedBy string `json:"verifiedBy,omitempty"` // fingerprint of the key that verified
	Error      string `json:"error,omitempty"`
	// KeyFingerprint is the fingerprint of the key the package carries, set even
	// when the signature does not verify; empty when the package has no usable key.
	KeyFingerprint string `json:"keyFingerprint,omitempty"`
	// Publisher is the trusted publisher's name; empty unless Trusted.
	Publisher string `json:"publisher,omitempty"`
	// Trusted says the verified key is in the instance's trusted-publisher list.
	Trusted bool `json:"trusted,omitempty"`
	// Continuity compares the key with the one pinned on the installed version:
	// "" (no installed version, or it was unsigned), "same", "changed", "lost".
	Continuity string `json:"continuity,omitempty"`
	// KeyPEM is the package's public key, kept so an admin can trust it later
	// without the ZIP. Persisted in its own column, never in the signature JSON.
	KeyPEM string `json:"-"`
}

// Signature errors that mean "nothing was signed", as opposed to a signature
// that was presented and failed.
const (
	SigErrNoKey       = "package carries no signing key"
	SigErrNoSignature = "no signature"
)

// Continuity values of SignatureStatus.Continuity.
const (
	ContinuitySame    = "same"
	ContinuityChanged = "changed"
	ContinuityLost    = "lost"
)

// Unsigned reports a package that presents no signature at all (no key, or a
// key without a detached signature), distinct from an invalid one.
func (s *SignatureStatus) Unsigned() bool {
	return s != nil && !s.Verified && (s.Error == SigErrNoKey || s.Error == SigErrNoSignature)
}

// Signature verdict strings shared by the update preview and the install preview.
const (
	SigStateSigned           = "signed"
	SigStateSignedUntrusted  = "signed-untrusted"
	SigStateUnsigned         = "unsigned"
	SigStateInvalid          = "invalid"
	SigStatePublisherChanged = "publisher-changed"
	SigStateSignatureLost    = "signature-lost"
)

// State is the single verdict string for a status; "" when s is nil (policy off).
func (s *SignatureStatus) State() string {
	switch {
	case s == nil:
		return ""
	case s.Continuity == ContinuityChanged:
		return SigStatePublisherChanged
	case s.Continuity == ContinuityLost:
		return SigStateSignatureLost
	case s.Verified && s.Trusted:
		return SigStateSigned
	case s.Verified:
		return SigStateSignedUntrusted
	case s.Unsigned():
		return SigStateUnsigned
	default:
		return SigStateInvalid
	}
}

// ApplyContinuity sets s.Continuity against the key pinned on the installed
// version. A first install, or an installed version that was never signed,
// has no continuity to keep.
func (s *SignatureStatus) ApplyContinuity(pinnedFingerprint string) {
	if s == nil || pinnedFingerprint == "" {
		return
	}
	switch {
	case !s.Verified:
		s.Continuity = ContinuityLost
	case s.KeyFingerprint == pinnedFingerprint:
		s.Continuity = ContinuitySame
	default:
		s.Continuity = ContinuityChanged
	}
}

// PluginSecuritySnapshot aggregates all security-relevant data for one plugin.
type PluginSecuritySnapshot struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	SandboxMode string            `json:"sandboxMode"`
	Permissions []string          `json:"permissions"`
	Findings    []SecurityFinding `json:"findings"`
	LastError   *PluginLastError  `json:"lastError,omitempty"`
	// Signature and SourceSHA256 are INSTALL-TIME facts: cosign runs on the
	// candidate ZIP when the admin installs it, and neither is re-checked when
	// the plugin starts. The install dir is writable by the plugin itself, so
	// these describe what was installed, never what is running. SizeBytes is the
	// only live value here.
	Signature     *SignatureStatus `json:"signature,omitempty"`
	SourceSHA256  string           `json:"sourceSHA256"`
	InstalledAt   time.Time        `json:"installedAt"`
	LastStartedAt *time.Time       `json:"lastStartedAt,omitempty"`
	SizeBytes     int64            `json:"sizeBytes"`
}
