package plugin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// preview.go holds the read-only preview surface: PreviewPlugin (manifest +
// analysis of a candidate ZIP) and PreviewUpdate (the permission/finding diff
// between an installed plugin and a candidate). Neither mutates any state.

// PluginPreview holds manifest info + analysis without installing.
type PluginPreview struct {
	Name         string
	Version      string
	Description  string
	Author       string
	Language     string
	Entrypoint   string
	Permissions  []string
	Capabilities []string
	Analysis     *plugin.AnalysisReport
	SourceHash   string
	// Signature is the cosign verdict on the candidate, nil when the policy is
	// off. SignatureState is its verdict string (plugin.SigState*), judged
	// against the installed plugin of the same name when there is one.
	Signature      *plugin.SignatureStatus
	SignatureState string
}

// PluginUpdatePreview holds the diff between an installed plugin and a
// candidate update ZIP. No state is mutated when building this struct.
type PluginUpdatePreview struct {
	CurrentVersion   string
	CandidateVersion string

	PermissionsDiff   PermissionDiff
	CapabilitiesDiff  PermissionDiff
	SlotsDiff         PermissionDiff
	ExternalFetchDiff PermissionDiff

	// NewFindings are analysis findings present in the candidate but not
	// critical-blocking (those would prevent the preview from being built).
	NewFindings []plugin.PluginFinding

	// SignatureStatus is the cosign verdict on the candidate (plugin.SigState*).
	// Empty string when sig verification is disabled.
	SignatureStatus string
	// Signature is the full verdict, with the continuity against the installed
	// version; nil when sig verification is disabled.
	Signature *plugin.SignatureStatus

	// RequiresAck is true when any permission/capability/slot/host was added,
	// or when the signing publisher changed or was lost.
	RequiresAck bool

	// SourceHash is the SHA-256 of the candidate ZIP. Must be passed back
	// in ApplyPluginUpdateRequest.AcknowledgedHash to prevent TOCTOU.
	SourceHash string
}

func (s *InstallerService) PreviewPlugin(ctx context.Context, zipPath string) (*PluginPreview, error) {
	manifest, err := s.parser.Parse(zipPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	sourceHash, hashErr := hashFile(zipPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash plugin file: %w", hashErr)
	}
	preview := &PluginPreview{
		Name:         manifest.Name,
		Version:      manifest.Version,
		Description:  manifest.Description,
		Author:       manifest.Author,
		Language:     manifest.Language,
		Entrypoint:   manifest.Entrypoint,
		Permissions:  manifest.Permissions,
		Capabilities: manifest.Capabilities,
		SourceHash:   sourceHash,
	}
	// An upload under an installed plugin's name is an update: judge its key
	// against the pinned one, as the install will.
	existing, lookupErr := s.plugins.GetByName(ctx, manifest.Name)
	if lookupErr != nil {
		existing = nil
	}
	preview.Signature = s.candidateSignature(ctx, zipPath, existing)
	preview.SignatureState = preview.Signature.State()
	if s.installer != nil && s.analyzer != nil && s.fs != nil {
		tmpDir, tmpErr := s.fs.MkdirTemp("", "evey-plugin-preview-*")
		if tmpErr == nil {
			defer func() {
				if rmErr := s.fs.RemoveAll(tmpDir); rmErr != nil {
					slog.Warn("remove preview tmpdir", slog.Any("err", rmErr))
				}
			}()
			if extErr := s.installer.Extract(zipPath, tmpDir); extErr == nil {
				report, analyzeErr := s.analyzer.Analyze(tmpDir, manifest.WebRoot)
				if analyzeErr == nil {
					preview.Analysis = report
				}
			}
		}
	}
	return preview, nil
}

// PreviewUpdate builds a PluginUpdatePreview by comparing the currently
// installed plugin to the candidate ZIP. No state is mutated. The returned
// SourceHash must be echoed back in ApplyUpdate as AcknowledgedHash to
// prevent TOCTOU attacks.
func (s *InstallerService) PreviewUpdate(ctx context.Context, pluginID, source string) (*PluginUpdatePreview, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.PreviewUpdate")
	defer span.End()
	existing, err := s.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return nil, ErrPluginNotFound
	}

	localPath, _, cleanupDL, dlErr := s.downloadIfURL(ctx, source)
	defer cleanupDL()
	if dlErr != nil {
		return nil, fmt.Errorf("resolve plugin source: %w", dlErr)
	}

	manifest, parseErr := s.parser.Parse(localPath)
	if parseErr != nil {
		return nil, fmt.Errorf("parse manifest: %w", parseErr)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	sourceHash, hashErr := hashFile(localPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash candidate: %w", hashErr)
	}

	permDiff := DiffPermissions(existing.Permissions, manifest.Permissions)
	capDiff := DiffCapabilities(existing.Capabilities, manifest.Capabilities)
	slotDiff := DiffSlots(existing.Frontend, manifest.Frontend)
	fetchDiff := DiffExternalFetch(existing.ExternalFetch, manifest.ExternalFetch)

	requiresAck := len(permDiff.Added) > 0 || len(capDiff.Added) > 0 ||
		len(slotDiff.Added) > 0 || len(fetchDiff.Added) > 0

	preview := &PluginUpdatePreview{
		CurrentVersion:    existing.Version,
		CandidateVersion:  manifest.Version,
		PermissionsDiff:   permDiff,
		CapabilitiesDiff:  capDiff,
		SlotsDiff:         slotDiff,
		ExternalFetchDiff: fetchDiff,
		RequiresAck:       requiresAck,
		SourceHash:        sourceHash,
	}

	// Run analyzer on candidate in a temp dir (best-effort - errors don't block preview).
	if s.installer != nil && s.analyzer != nil && s.fs != nil {
		tmpDir, tmpErr := s.fs.MkdirTemp("", "evey-plugin-update-preview-*")
		if tmpErr == nil {
			defer func() {
				if rmErr := s.fs.RemoveAll(tmpDir); rmErr != nil {
					slog.WarnContext(ctx, "remove update preview tmpdir", slog.Any("err", rmErr))
				}
			}()
			if extErr := s.installer.Extract(localPath, tmpDir); extErr == nil {
				report, analyzeErr := s.analyzer.Analyze(tmpDir, manifest.WebRoot)
				if analyzeErr == nil {
					preview.NewFindings = report.RawFindings
				}
			}
		}
	}

	// Cosign on candidate (best-effort - the verdict is recorded, not blocking).
	if sig := s.candidateSignature(ctx, localPath, existing); sig != nil {
		preview.Signature = sig
		preview.SignatureStatus = sig.State()
		if sig.Continuity == plugin.ContinuityChanged || sig.Continuity == plugin.ContinuityLost {
			preview.RequiresAck = true
		}
	}

	return preview, nil
}

// candidateSignature verifies a candidate ZIP and sets its continuity against
// the installed plugin (nil for a first install). Returns nil when verification
// is disabled and nothing is pinned. An enforce-policy refusal still yields a
// status, so the admin sees why instead of a bare error.
func (s *InstallerService) candidateSignature(ctx context.Context, zipPath string, existing *plugin.Plugin) *plugin.SignatureStatus {
	if s.sigVerifier == nil {
		return nil
	}
	st, err := s.candidateSignatureStatus(ctx, zipPath, existing)
	if st == nil {
		if err == nil {
			return nil
		}
		st = &plugin.SignatureStatus{Error: err.Error()}
	}
	if existing != nil {
		st.ApplyContinuity(existing.PublisherKeyFingerprint)
	}
	return st
}

// PreviewSource is PreviewPlugin for an install SOURCE - a URL or a local
// path, exactly what InstallPlugin accepts. Installing from a URL used to
// skip the preview entirely (the web page said so in so many words): the
// admin saw the manifest, the warnings and the outbound URLs only AFTER the
// plugin was installed and running. This downloads the same archive the
// install would, previews it, and throws it away; a "#sha256:" fragment on
// the URL is checked against the archive, since that is the only reason a
// caller writes one.
func (s *InstallerService) PreviewSource(ctx context.Context, source string) (*PluginPreview, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.PreviewSource")
	defer span.End()
	localPath, expectedHash, cleanupDL, dlErr := s.downloadIfURL(ctx, source)
	defer cleanupDL()
	if dlErr != nil {
		return nil, fmt.Errorf("resolve plugin source: %w", dlErr)
	}
	preview, err := s.PreviewPlugin(ctx, localPath)
	if err != nil {
		return nil, err
	}
	if expectedHash != "" && preview.SourceHash != expectedHash {
		return nil, fmt.Errorf("plugin hash mismatch: expected %s, got %s", expectedHash, preview.SourceHash)
	}
	return preview, nil
}
