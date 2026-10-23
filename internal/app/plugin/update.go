package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// update.go holds the in-place upgrade surface: applying a previewed update
// (with the TOCTOU hash re-check) and the permission-escalation guard that an
// update must pass unless the admin explicitly acknowledged the diff.

// ErrPluginPermissionEscalation is returned when an update would silently
// grant the plugin new permissions, capabilities, frontend slots, or
// external_fetch entries that were not present in the installed version.
// The caller (admin handler) must pass WithAllowPermissionEscalation(ctx)
// after an explicit admin acknowledgment to proceed.
var ErrPluginPermissionEscalation = errors.New("plugin update requires permission escalation")

type permEscalationCtxKey struct{}

// WithAllowPermissionEscalation returns a child context that authorizes an
// update to proceed even when it adds new permissions/capabilities/slots/hosts.
// Call this only after the admin has explicitly acknowledged the diff.
func WithAllowPermissionEscalation(ctx context.Context) context.Context {
	return context.WithValue(ctx, permEscalationCtxKey{}, true)
}

func allowPermissionEscalation(ctx context.Context) bool {
	v, _ := ctx.Value(permEscalationCtxKey{}).(bool)
	return v
}

// ErrPublisherChanged and ErrSignatureLost are returned when an update is signed
// by another key than the installed version, or is no longer validly signed.
// Only an admin who read the verdict (accept_new_publisher) may take it; the
// key pinned at the first install is what stops a hijacked source URL from
// swapping in code signed by someone else.
var (
	ErrPublisherChanged = errors.New("plugin update is signed by a different publisher key than the installed version")
	ErrSignatureLost    = errors.New("plugin update is no longer validly signed, the installed version was")
)

type acceptPublisherCtxKey struct{}

// WithAcceptNewPublisher returns a child context that lets an update change or
// drop the publisher key pinned on the installed version. Call this only after
// the admin has explicitly accepted the new key.
func WithAcceptNewPublisher(ctx context.Context) context.Context {
	return context.WithValue(ctx, acceptPublisherCtxKey{}, true)
}

func acceptNewPublisher(ctx context.Context) bool {
	v, _ := ctx.Value(acceptPublisherCtxKey{}).(bool)
	return v
}

// ApplyUpdate applies a previously previewed update. It downloads (or reads) the
// candidate ONCE, checks its hash against acknowledged_hash, then installs that
// same file through the full install pipeline (TOCTOU: a second fetch could serve
// other bytes), with WithAllowPermissionEscalation to honour the admin's ack.
func (s *InstallerService) ApplyUpdate(ctx context.Context, pluginID, source, acknowledgedHash string, acceptPublisher bool) (*plugin.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "app.plugin.ApplyUpdate")
	defer span.End()
	// Verify the plugin exists before going further.
	if _, err := s.plugins.GetByID(ctx, pluginID); err != nil {
		return nil, ErrPluginNotFound
	}

	// Download/read the candidate once and hash it; this file is what gets installed.
	localPath, fragmentHash, cleanupDL, dlErr := s.downloadIfURL(ctx, source)
	defer cleanupDL()
	if dlErr != nil {
		return nil, fmt.Errorf("resolve plugin source: %w", dlErr)
	}

	freshHash, hashErr := hashFile(localPath)
	if hashErr != nil {
		return nil, fmt.Errorf("hash candidate: %w", hashErr)
	}

	if freshHash != acknowledgedHash {
		return nil, fmt.Errorf("%w: expected %s got %s", ErrCandidateChanged, acknowledgedHash, freshHash)
	}

	// Permission escalation is explicitly authorised because the admin has
	// reviewed and acknowledged the diff in the preview step.
	ctx = WithAllowPermissionEscalation(ctx)
	if acceptPublisher {
		ctx = WithAcceptNewPublisher(ctx)
	}
	if fragmentHash == "" {
		fragmentHash = acknowledgedHash
	}
	return s.installLocalZip(ctx, localPath, fragmentHash)
}

// checkPermissionEscalation returns ErrPluginPermissionEscalation when the
// incoming manifest requests permissions, capabilities, frontend slots, or
// external_fetch entries that are absent from the currently installed plugin.
// Same-or-reduced sets are allowed without escalation.
func checkPermissionEscalation(manifest *plugin.PluginManifest, existing *plugin.Plugin) error {
	existingPerms := sliceToSet(existing.Permissions)
	existingCaps := sliceToSet(existing.Capabilities)
	existingHosts := sliceToSet(existing.ExternalFetch)

	existingSlots := make(map[string]struct{}, len(existing.Frontend))
	for _, f := range existing.Frontend {
		existingSlots[f.Slot] = struct{}{}
	}

	var added []string
	for _, p := range manifest.Permissions {
		if _, ok := existingPerms[p]; !ok {
			added = append(added, "permission:"+p)
		}
	}
	for _, c := range manifest.Capabilities {
		if _, ok := existingCaps[c]; !ok {
			added = append(added, "capability:"+c)
		}
	}
	for _, h := range manifest.ExternalFetch {
		if _, ok := existingHosts[h]; !ok {
			added = append(added, "external_fetch:"+h)
		}
	}
	for _, f := range manifest.Frontend {
		if _, ok := existingSlots[f.Slot]; !ok {
			added = append(added, "slot:"+f.Slot)
		}
	}

	if len(added) > 0 {
		return fmt.Errorf("%w: new entries requested: %s", ErrPluginPermissionEscalation, strings.Join(added, ", "))
	}
	return nil
}
