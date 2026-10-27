package grpcapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appconfig "github.com/eveyproject/evey/internal/app/config"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// pluginResourceController is the subset of the plugin runner the
// resource-limits RPCs drive: read the live caps and push updated ones. Kept as
// an interface so grpcapi stays decoupled from the pluginrunner adapter.
// *pluginrunner.Runner satisfies it.
type pluginResourceController interface {
	appplugin.ResourceController
	ResourceConfig() (memMB, pidsMax, diskMB, nice int, sandbox string)
}

type adminPluginHandler struct {
	eveyv1.UnimplementedAdminPluginServiceServer
	pluginInstaller *appplugin.InstallerService
	pluginRuntime   *appplugin.RuntimeService
	pluginQuery     *appplugin.QueryService
	tokenSvc        *appplugin.TokenService
	logDir          string
	translator      *eveyi18n.Translator
	// resource-limits RPCs: the runner (live caps), the server-config store
	// (persist + publish; also read back for the effective values), and the
	// env/boot defaults used when a cap has no stored override. pluginRes is nil
	// when the plugin system is disabled (the runner is not constructed).
	pluginRes   pluginResourceController
	serverCfg   shared.ServerConfigRepository
	resDefaults appplugin.ResourceDefaults
	// enabled mirrors cfg.Server.PluginsEnabled. When false, every
	// install/enable/disable/token/runtime mutation returns
	// FailedPrecondition. Listing/reading is left open so the admin UI
	// can surface "plugins disabled" instead of breaking the page.
	enabled bool
	// metadata plugins: the fields each may replace and its library walk.
	// nil when not wired (the RPCs then answer FailedPrecondition).
	metadataSettings pluginMetadataSettings
	metadataRefresh  pluginMetadataRefresher
}

// requireEnabled gates state-changing or executing plugin operations.
// Read-only methods (List*, Get*) intentionally bypass this so the
// admin UI can render a consistent "plugins disabled" state without
// 500-ing.
func (d *adminPluginHandler) requireEnabled(ctx context.Context) error {
	if !d.enabled {
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.system_disabled")
	}
	return nil
}

func (d *adminPluginHandler) IssuePluginToken(ctx context.Context, req *eveyv1.IssuePluginTokenRequest) (*eveyv1.IssuePluginTokenResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.IssuePluginToken")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if req.PluginId == "" {
		span.SetStatus(otelcodes.Error, "plugin_id required")
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.id_required")
	}
	if d.tokenSvc == nil {
		span.SetStatus(otelcodes.Error, "plugin token service not configured")
		return nil, status.Error(codes.Unimplemented, "plugin token service not configured")
	}
	issued, err := d.tokenSvc.IssueToken(ctx, claims.UserID, claims.ProfileID, req.PluginId)
	if err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appplugin.ErrPluginNotFound):
			span.SetStatus(otelcodes.Error, "plugin not found")
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
		case errors.Is(err, appplugin.ErrPluginNotEnabled):
			span.SetStatus(otelcodes.Error, "plugin not enabled")
			return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.not_enabled")
		default:
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "issue plugin token", err)
		}
	}
	return &eveyv1.IssuePluginTokenResponse{
		Token:     issued.Token,
		ExpiresAt: timestamppb.New(issued.ExpiresAt),
		Scopes:    issued.Scopes,
	}, nil
}

func (d *adminPluginHandler) ListPluginRuntimes(ctx context.Context, _ *eveyv1.ListPluginRuntimesRequest) (*eveyv1.ListPluginRuntimesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ListPluginRuntimes")
	defer span.End()
	runtimes, err := d.pluginQuery.ListRuntimes(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list runtimes", err)
	}
	resp := &eveyv1.ListPluginRuntimesResponse{}
	for _, rt := range runtimes {
		resp.Runtimes = append(resp.Runtimes, pluginRuntimeToProto(rt))
	}
	return resp, nil
}

func (d *adminPluginHandler) AddPluginRuntime(ctx context.Context, req *eveyv1.AddPluginRuntimeRequest) (*eveyv1.PluginRuntime, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.AddPluginRuntime")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	rt := &plugin.PluginRuntime{
		ID:       req.Name,
		Language: req.Language,
		Version:  req.Version,
	}
	if err := d.pluginRuntime.AddRuntime(ctx, rt); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "add runtime", err)
	}
	return pluginRuntimeToProto(rt), nil
}

func (d *adminPluginHandler) RemovePluginRuntime(ctx context.Context, req *eveyv1.RemovePluginRuntimeRequest) (*eveyv1.RemovePluginRuntimeResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RemovePluginRuntime")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := d.pluginRuntime.RemoveRuntime(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if errors.Is(err, shared.ErrNotFound) {
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.runtime_not_found")
		}
		return nil, internalError(ctx, "remove runtime", err)
	}
	return &eveyv1.RemovePluginRuntimeResponse{}, nil
}

func (d *adminPluginHandler) InstallPlugin(ctx context.Context, req *eveyv1.InstallPluginRequest) (*eveyv1.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.InstallPlugin")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	// When the admin has explicitly acknowledged the permission diff in the
	// UI, the request carries accept_permission_changes=true and we authorize
	// the installer to proceed even when the update widens permissions /
	// capabilities / slots / external_fetch hosts.
	if req.GetAcceptPermissionChanges() {
		ctx = appplugin.WithAllowPermissionEscalation(ctx)
	}
	p, err := d.pluginInstaller.InstallPlugin(ctx, req.Source)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		// Surface permission escalation as FAILED_PRECONDITION so the
		// frontend can detect it and show the diff modal. The error text
		// already contains the "new entries requested: ..." breakdown.
		if errors.Is(err, appplugin.ErrPluginPermissionEscalation) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, d.installPluginError(ctx, err)
	}
	return pluginToProto(d.withLiveTrust(ctx, p)), nil
}

// installPluginError separates a refused archive (the caller's problem, said
// with its reason - the upload route already does) from a server fault.
func (d *adminPluginHandler) installPluginError(ctx context.Context, err error) error {
	var blocked *appplugin.PluginBlockedError
	switch {
	case errors.As(err, &blocked), strings.Contains(err.Error(), "plugin blocked:"):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, appplugin.ErrInvalidManifest), errors.Is(err, appplugin.ErrHashMismatch):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, appplugin.ErrSourceUnavailable):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, appplugin.ErrPublisherChanged):
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.publisher_changed")
	case errors.Is(err, appplugin.ErrSignatureLost):
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.signature_lost")
	}
	return internalError(ctx, "install plugin", err)
}

func (d *adminPluginHandler) UninstallPlugin(ctx context.Context, req *eveyv1.UninstallPluginRequest) (*eveyv1.UninstallPluginResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.UninstallPlugin")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := d.pluginInstaller.UninstallPlugin(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if errors.Is(err, appplugin.ErrPluginNotFound) {
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
		}
		return nil, internalError(ctx, "uninstall plugin", err)
	}
	return &eveyv1.UninstallPluginResponse{}, nil
}

func (d *adminPluginHandler) EnablePlugin(ctx context.Context, req *eveyv1.EnablePluginRequest) (*eveyv1.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.EnablePlugin")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := d.pluginRuntime.EnablePlugin(ctx, req.Id); err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appplugin.ErrPluginNotFound):
			span.SetStatus(otelcodes.Error, "plugin not found")
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
		case errors.Is(err, appplugin.ErrAlreadyRunning):
			span.SetStatus(otelcodes.Error, "plugin already running")
			return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.already_running")
		default:
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "enable plugin", err)
		}
	}
	return &eveyv1.Plugin{Id: req.Id, Enabled: true}, nil
}

func (d *adminPluginHandler) DisablePlugin(ctx context.Context, req *eveyv1.DisablePluginRequest) (*eveyv1.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.DisablePlugin")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := d.pluginRuntime.DisablePlugin(ctx, req.Id); err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appplugin.ErrPluginNotFound):
			span.SetStatus(otelcodes.Error, "plugin not found")
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
		case errors.Is(err, appplugin.ErrNotRunning):
			span.SetStatus(otelcodes.Error, "plugin not running")
			return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.not_running")
		default:
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "disable plugin", err)
		}
	}
	return &eveyv1.Plugin{Id: req.Id, Enabled: false}, nil
}

func (d *adminPluginHandler) ListPlugins(ctx context.Context, _ *eveyv1.ListPluginsRequest) (*eveyv1.ListPluginsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ListPlugins")
	defer span.End()
	plugins, err := d.pluginQuery.ListPlugins(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list plugins", err)
	}
	resp := &eveyv1.ListPluginsResponse{}
	for _, p := range plugins {
		resp.Plugins = append(resp.Plugins, pluginToProto(d.withLiveTrust(ctx, p)))
	}
	return resp, nil
}

// publisherService is the trusted-publisher keyring, nil when not wired.
func (d *adminPluginHandler) publisherService() *appplugin.TrustedPublisherService {
	if d.pluginInstaller == nil {
		return nil
	}
	return d.pluginInstaller.Publishers()
}

// withLiveTrust returns p with its install-time signature verdict re-judged
// against the current keyring (name, trusted). The stored plugin is not touched.
func (d *adminPluginHandler) withLiveTrust(ctx context.Context, p *plugin.Plugin) *plugin.Plugin {
	pub := d.publisherService()
	if pub == nil || p.Signature == nil {
		return p
	}
	cp := *p
	cp.Signature = appplugin.RefreshTrust(ctx, p.Signature, pub.Lookup)
	return &cp
}

func (d *adminPluginHandler) ListTrustedPublishers(ctx context.Context, _ *eveyv1.ListTrustedPublishersRequest) (*eveyv1.ListTrustedPublishersResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ListTrustedPublishers")
	defer span.End()
	resp := &eveyv1.ListTrustedPublishersResponse{}
	pub := d.publisherService()
	if pub == nil {
		return resp, nil
	}
	list, err := pub.List(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list trusted publishers", err)
	}
	for i := range list {
		resp.Publishers = append(resp.Publishers, trustedPublisherToProto(&list[i]))
	}
	return resp, nil
}

func (d *adminPluginHandler) AddTrustedPublisher(ctx context.Context, req *eveyv1.AddTrustedPublisherRequest) (*eveyv1.TrustedPublisher, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.AddTrustedPublisher")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	pub := d.publisherService()
	if pub == nil {
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.plugin.publishers_unavailable")
	}
	var (
		added *plugin.TrustedPublisher
		err   error
	)
	hasPEM, hasFP := strings.TrimSpace(req.PublicKeyPem) != "", strings.TrimSpace(req.Fingerprint) != ""
	switch {
	case hasPEM == hasFP:
		err = appplugin.ErrPublisherKeyRequired
	case hasPEM:
		added, err = pub.AddByPEM(ctx, req.Name, req.PublicKeyPem)
	default:
		added, err = pub.AddByFingerprint(ctx, req.Name, req.Fingerprint)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, appplugin.ErrPublisherNameRequired):
			return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.publisher_name_required")
		case errors.Is(err, appplugin.ErrPublisherNameTooLong):
			return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.publisher_name_too_long")
		case errors.Is(err, appplugin.ErrPublisherKeyRequired):
			return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.publisher_key_required")
		case errors.Is(err, appplugin.ErrPublisherKeyInvalid):
			return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.publisher_key_invalid")
		case errors.Is(err, appplugin.ErrPublisherKeyUnknown):
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.publisher_key_unknown")
		}
		return nil, internalError(ctx, "add trusted publisher", err)
	}
	return trustedPublisherToProto(added), nil
}

func (d *adminPluginHandler) RemoveTrustedPublisher(ctx context.Context, req *eveyv1.RemoveTrustedPublisherRequest) (*eveyv1.RemoveTrustedPublisherResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RemoveTrustedPublisher")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	pub := d.publisherService()
	if pub == nil {
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.plugin.publishers_unavailable")
	}
	if err := pub.Remove(ctx, req.Fingerprint); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, shared.ErrNotFound):
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.publisher_not_found")
		case errors.Is(err, plugin.ErrSeededPublisher):
			return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.publisher_seeded")
		}
		return nil, internalError(ctx, "remove trusted publisher", err)
	}
	return &eveyv1.RemoveTrustedPublisherResponse{}, nil
}

func (d *adminPluginHandler) GetPluginConfig(ctx context.Context, req *eveyv1.GetPluginConfigRequest) (*eveyv1.PluginConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetPluginConfig")
	defer span.End()
	result, err := d.pluginQuery.GetPluginConfig(ctx, req.PluginId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "get config")
		return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
	}
	resp := &eveyv1.PluginConfig{
		PluginId: req.PluginId,
		Values:   maskedPluginConfig(result.Config, result.ConfigSchema),
	}
	if len(result.ConfigSchema) > 0 {
		schemaJSON, err := json.Marshal(result.ConfigSchema)
		if err == nil {
			resp.ConfigSchemaJson = string(schemaJSON)
		}
	}
	return resp, nil
}

func (d *adminPluginHandler) UpdatePluginConfig(ctx context.Context, req *eveyv1.UpdatePluginConfigRequest) (*eveyv1.PluginConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.UpdatePluginConfig")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	result, err := d.pluginQuery.GetPluginConfig(ctx, req.PluginId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "get plugin")
		return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
	}
	values := restoreMaskedSecrets(req.Values, result.Config, result.ConfigSchema)
	if err := validateConfigValues(values, result.ConfigSchema); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	cfg := coerceConfigValues(values, result.ConfigSchema)
	if err := d.pluginRuntime.UpdatePluginConfig(ctx, req.PluginId, cfg); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "update config", err)
	}
	return &eveyv1.PluginConfig{
		PluginId: req.PluginId,
		Values:   maskedPluginConfig(cfg, result.ConfigSchema),
	}, nil
}

// maskedPluginConfig renders a config for a client: secret fields (password
// type or `secret: true`) come back as SecretMask, empty ones stay empty.
func maskedPluginConfig(cfg, schema map[string]any) map[string]string {
	out := make(map[string]string, len(cfg))
	for k, v := range cfg {
		s := stringFromInterface(v)
		if s != "" && plugin.IsSecretConfigField(schema[k]) {
			s = appconfig.SecretMask
		}
		out[k] = s
	}
	return out
}

// restoreMaskedSecrets swaps a secret field echoed back as SecretMask for the
// stored value - the admin form posts every field on save, so the mask means
// "unchanged", never a new credential.
func restoreMaskedSecrets(values map[string]string, stored, schema map[string]any) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		if v == appconfig.SecretMask && plugin.IsSecretConfigField(schema[k]) {
			v = stringFromInterface(stored[k])
		}
		out[k] = v
	}
	return out
}

func (d *adminPluginHandler) PreviewPluginUpdate(ctx context.Context, req *eveyv1.PreviewPluginUpdateRequest) (*eveyv1.PreviewPluginUpdateResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.PreviewPluginUpdate")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if req.PluginId == "" {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.id_required")
	}
	if req.Source == "" {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.source_required")
	}
	preview, err := d.pluginInstaller.PreviewUpdate(ctx, req.PluginId, req.Source)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if errors.Is(err, appplugin.ErrPluginNotFound) {
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
		}
		return nil, internalError(ctx, "preview update", err)
	}
	resp := &eveyv1.PreviewPluginUpdateResponse{
		CurrentVersion:       preview.CurrentVersion,
		CandidateVersion:     preview.CandidateVersion,
		PermissionsAdded:     preview.PermissionsDiff.Added,
		PermissionsRemoved:   preview.PermissionsDiff.Removed,
		CapabilitiesAdded:    preview.CapabilitiesDiff.Added,
		CapabilitiesRemoved:  preview.CapabilitiesDiff.Removed,
		SlotsAdded:           preview.SlotsDiff.Added,
		SlotsRemoved:         preview.SlotsDiff.Removed,
		ExternalFetchAdded:   preview.ExternalFetchDiff.Added,
		ExternalFetchRemoved: preview.ExternalFetchDiff.Removed,
		SignatureStatus:      preview.SignatureStatus,
		Signature:            signatureToProto(preview.Signature),
		RequiresAck:          preview.RequiresAck,
		SourceHash:           preview.SourceHash,
	}
	for _, f := range preview.NewFindings {
		resp.NewFindings = append(resp.NewFindings, &eveyv1.SecurityFinding{
			Category:    f.Category,
			Severity:    f.Severity,
			File:        f.File,
			Line:        int32(f.Line),
			Description: f.Description,
		})
	}
	return resp, nil
}

func (d *adminPluginHandler) ApplyPluginUpdate(ctx context.Context, req *eveyv1.ApplyPluginUpdateRequest) (*eveyv1.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ApplyPluginUpdate")
	defer span.End()
	if err := d.requireEnabled(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if req.PluginId == "" {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.id_required")
	}
	if req.Source == "" {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.source_required")
	}
	if req.AcknowledgedHash == "" {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.acknowledged_hash_required")
	}
	p, err := d.pluginInstaller.ApplyUpdate(ctx, req.PluginId, req.Source, req.AcknowledgedHash, req.AcceptNewPublisher)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, appplugin.ErrPluginNotFound):
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
		case errors.Is(err, appplugin.ErrCandidateChanged):
			return nil, d.translator.TranslatedError(ctx, codes.Aborted, "errors.plugin.candidate_changed")
		case errors.Is(err, appplugin.ErrPublisherChanged):
			return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.publisher_changed")
		case errors.Is(err, appplugin.ErrSignatureLost):
			return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.signature_lost")
		default:
			return nil, internalError(ctx, "apply update", err)
		}
	}
	return pluginToProto(d.withLiveTrust(ctx, p)), nil
}

func (d *adminPluginHandler) GetPluginLogs(ctx context.Context, req *eveyv1.GetPluginLogsRequest) (*eveyv1.GetPluginLogsResponse, error) {
	_, span := tracing.StartSpan(ctx, "grpc.admin.GetPluginLogs")
	defer span.End()
	if d.logDir == "" || req.PluginId == "" {
		return &eveyv1.GetPluginLogsResponse{}, nil
	}
	// Plugin ids are server-generated ("plg-<hex>", see id.New) and never carry
	// a separator, so anything that does is a traversal attempt: the id used to
	// go straight into the join, letting an admin read any *.log on the host.
	// The plugin ASSET handler already rejects exactly these - this is its twin.
	if strings.ContainsAny(req.PluginId, `/\`) || strings.Contains(req.PluginId, "..") {
		span.SetStatus(otelcodes.Error, "invalid plugin id")
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.id_invalid")
	}
	paths, err := pluginLogFiles(filepath.Join(d.logDir, "plugins"), req.PluginId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list logs", err)
	}

	filterLevel := strings.ToUpper(req.Level)
	var all []*eveyv1.PluginLogEntry
	for _, logPath := range paths {
		entries, readErr := readPluginLogFile(logPath, filterLevel)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue // rotation/purge concurrente
			}
			span.RecordError(readErr)
			span.SetStatus(otelcodes.Error, readErr.Error())
			return nil, internalError(ctx, "open log", readErr)
		}
		all = append(all, entries...)
	}

	total := int32(len(all))
	offset := max(int(req.Offset), 0)
	limit := int(req.Limit)
	if offset >= len(all) {
		return &eveyv1.GetPluginLogsResponse{Total: total}, nil
	}
	// Sans offset explicite, un "limit" tronque la FIN et non le début : on lit
	// des logs pour voir ce qui vient de se passer. Le panel admin demande
	// limit=100 sans offset - il aurait affiché les 100 plus VIEILLES lignes,
	// c'est-à-dire potentiellement plusieurs jours avant la panne à diagnostiquer.
	// Total reste le compte complet, donc la pagination explicite (offset > 0)
	// garde sa sémantique chronologique habituelle.
	if offset == 0 && limit > 0 && limit < len(all) {
		return &eveyv1.GetPluginLogsResponse{Entries: all[len(all)-limit:], Total: total}, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return &eveyv1.GetPluginLogsResponse{Entries: all, Total: total}, nil
}

// pluginLogFiles liste les fichiers de log d'un plugin, du plus ancien au plus
// récent. PluginLogger écrit "<id>-YYYY-MM-DD.log" (plus "<id>-YYYY-MM-DD-HH.log"
// quand la taille force une rotation intra-journée) : le lecteur cherchait
// "<id>.log", un nom que rien n'écrit, donc le visualiseur du panel admin
// renvoyait total=0 en permanence. Le nom nu reste accepté par compatibilité.
// Volontairement ReadDir + préfixe plutôt que filepath.Glob : un pluginID n'est
// validé que contre les séparateurs et "..", donc un motif comme "plg-*" passerait
// la garde et ferait fusionner les logs de TOUS les plugins - exactement la
// confusion de périmètre que la garde de traversée existante cherche à empêcher.
func pluginLogFiles(dir, pluginID string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // aucun plugin n'a encore logué
		}
		return nil, err
	}

	// os.ReadDir trie par nom, et pour des noms datés (YYYY-MM-DD, puis -HH pour
	// les rotations de taille intra-journée) l'ordre lexicographique EST l'ordre
	// chronologique.
	var dated []string
	var plain string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch {
		case name == pluginID+".log":
			plain = filepath.Join(dir, name) // nom nu : gardé par compatibilité
		case strings.HasPrefix(name, pluginID+"-") && strings.HasSuffix(name, ".log"):
			dated = append(dated, filepath.Join(dir, name))
		}
	}
	if plain != "" {
		dated = append([]string{plain}, dated...)
	}
	return dated, nil
}

// readPluginLogFile lit un fichier de log plugin. Trois formes de lignes
// coexistent dans le MÊME fichier et doivent toutes ressortir :
//   - JSON structuré (plugins qui loguent en JSON),
//   - "<RFC3339> [LEVEL] message" (les lignes écrites par PluginLogger.WriteLog),
//   - stdout/stderr brut du plugin, sans aucune structure - c'est la majorité du
//     volume, et c'est précisément ce qui manquait à l'admin pendant l'incident
//     weather-v2 : le "listening on ..." du plugin prouvait que le process
//     tournait, alors que le serveur affichait un timeout de socket.
func readPluginLogFile(path, filterLevel string) ([]*eveyv1.PluginLogEntry, error) {
	f, err := os.Open(path) //nolint:gosec // chemin construit à partir d'un pluginID validé (pas de séparateur ni "..")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []*eveyv1.PluginLogEntry
	sc := bufio.NewScanner(f)
	// Une ligne de stdout de plugin peut être longue (stack trace sur une ligne) :
	// le buffer par défaut de 64 KiB ferait échouer le scan et tronquerait la
	// suite du fichier en silence.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := parsePluginLogLine(line)
		if filterLevel != "" && !strings.EqualFold(entry.Level, filterLevel) {
			continue
		}
		entries = append(entries, entry)
	}
	if scErr := sc.Err(); scErr != nil {
		return nil, scErr
	}
	return entries, nil
}

func parsePluginLogLine(line string) *eveyv1.PluginLogEntry {
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		if entry, ok := parseJSONLogLine(line); ok {
			return entry
		}
	}

	entry := &eveyv1.PluginLogEntry{Fields: map[string]string{}}
	rest := line
	// Horodatage en tête, tel que posé par WriteLog.
	if ts, after, ok := strings.Cut(line, " "); ok {
		if t, tErr := time.Parse(time.RFC3339, ts); tErr == nil {
			entry.Timestamp = timestamppb.New(t)
			rest = after
		}
	}
	// Niveau entre crochets, tel que posé par WriteLog / la plupart des plugins.
	if strings.HasPrefix(rest, "[") {
		if lvl, after, ok := strings.Cut(rest[1:], "]"); ok && !strings.ContainsAny(lvl, " \t") {
			entry.Level = strings.ToUpper(lvl)
			rest = strings.TrimPrefix(after, " ")
		}
	}
	entry.Message = rest
	return entry
}

func parseJSONLogLine(line string) (*eveyv1.PluginLogEntry, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, false
	}
	var level, msg string
	var ts time.Time
	if v, ok := raw["level"]; ok {
		json.Unmarshal(v, &level) //nolint:errcheck
	}
	if v, ok := raw["msg"]; ok {
		json.Unmarshal(v, &msg) //nolint:errcheck
	}
	if v, ok := raw["time"]; ok {
		var t string
		if json.Unmarshal(v, &t) == nil {
			ts, _ = time.Parse(time.RFC3339Nano, t) //nolint:errcheck // best-effort parse, zero time is acceptable
		}
	}
	fields := make(map[string]string)
	for k, v := range raw {
		if k == "level" || k == "msg" || k == "time" || k == "plugin_id" {
			continue
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			fields[k] = s
		}
	}
	entry := &eveyv1.PluginLogEntry{Level: level, Message: msg, Fields: fields}
	if !ts.IsZero() {
		entry.Timestamp = timestamppb.New(ts)
	}
	return entry, true
}

// GetPluginResourceLimits returns the per-plugin caps the runner is currently
// enforcing (env defaults overlaid with stored admin overrides).
func (d *adminPluginHandler) GetPluginResourceLimits(ctx context.Context, _ *eveyv1.GetPluginResourceLimitsRequest) (*eveyv1.PluginResourceLimits, error) {
	_, span := tracing.StartSpan(ctx, "grpc.admin.GetPluginResourceLimits")
	defer span.End()
	// Admin-only: requires a real user (a scoped plugin token carries no user
	// claims, so this rejects it on top of the RBAC + interceptor guards).
	if _, err := requireClaims(ctx); err != nil {
		return nil, err
	}
	return d.effectiveResourceLimits(ctx), nil
}

// UpdatePluginResourceLimits validates the requested caps, persists them to the
// server-config store (which also publishes a change event), and applies them
// to the runner synchronously - memory/PID/disk take effect on the next monitor
// sweep, cpu_nice and sandbox_mode on the next plugin start. Returns the new
// effective caps. Out-of-range values are rejected with InvalidArgument.
func (d *adminPluginHandler) UpdatePluginResourceLimits(ctx context.Context, req *eveyv1.PluginResourceLimits) (*eveyv1.PluginResourceLimits, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.UpdatePluginResourceLimits")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.general.request_required")
	}
	if req.MemoryMb < 0 || req.PidsMax < 0 || req.DiskMaxMb < 0 {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.limits_negative")
	}
	if req.CpuNice < 0 || req.CpuNice > 19 {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.cpu_nice_range")
	}
	if req.SandboxMode != "" && !appplugin.IsValidSandboxMode(req.SandboxMode) {
		return nil, d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.sandbox_mode_invalid", map[string]string{"modes": "auto, landlock, bubblewrap, strict, basic"})
	}
	if d.serverCfg == nil {
		return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.admin.service_not_configured")
	}

	// Capture the current sandbox mode so we can tell whether this update changes
	// it (and thus whether running plugins must be restarted to apply it).
	oldMode := ""
	if d.pluginRes != nil {
		_, _, _, _, oldMode = d.pluginRes.ResourceConfig()
	}

	updates := []struct{ key, val string }{
		{appplugin.KeyMemoryMB, strconv.Itoa(int(req.MemoryMb))},
		{appplugin.KeyPIDsMax, strconv.Itoa(int(req.PidsMax))},
		{appplugin.KeyDiskMaxMB, strconv.Itoa(int(req.DiskMaxMb))},
		{appplugin.KeyCPUNice, strconv.Itoa(int(req.CpuNice))},
		{appplugin.KeySandboxMode, req.SandboxMode},
	}
	for _, u := range updates {
		if err := d.serverCfg.Set(ctx, u.key, u.val); err != nil {
			span.RecordError(err)
			return nil, internalError(ctx, "persist plugin resource limit", err)
		}
	}
	// Apply synchronously: the change event is delivered asynchronously, so we
	// don't rely on it for our own response.
	appplugin.ApplyResourceConfig(ctx, d.serverCfg, d.pluginRes, d.resDefaults)

	// A sandbox-mode change must take effect now, not lazily on the next restart
	// (otherwise the dashboard shows the new mode while plugins still run under
	// the old one). Restart running plugins in the background; one that can't run
	// under the new mode is left in Error (visible), so the admin can revert.
	if d.pluginRes != nil && d.pluginRuntime != nil {
		if _, _, _, _, newMode := d.pluginRes.ResourceConfig(); newMode != oldMode {
			go d.pluginRuntime.RestartRunningPlugins(context.WithoutCancel(ctx))
		}
	}
	return d.effectiveResourceLimits(ctx), nil
}

// effectiveResourceLimits reports the caps the runner enforces. When the plugin
// system is disabled (no runner) it resolves the stored/overridden values
// directly so the admin UI still renders accurate numbers.
func (d *adminPluginHandler) effectiveResourceLimits(ctx context.Context) *eveyv1.PluginResourceLimits {
	if d.pluginRes != nil {
		mem, pids, disk, nice, sandbox := d.pluginRes.ResourceConfig()
		return &eveyv1.PluginResourceLimits{
			MemoryMb: int32(mem), PidsMax: int32(pids), DiskMaxMb: int32(disk),
			CpuNice: int32(nice), SandboxMode: sandbox,
		}
	}
	v := appplugin.ResolveResourceConfig(ctx, d.serverCfg, d.resDefaults)
	return &eveyv1.PluginResourceLimits{
		MemoryMb: int32(v.MemoryMB), PidsMax: int32(v.PIDsMax), DiskMaxMb: int32(v.DiskMaxMB),
		CpuNice: int32(v.CPUNice), SandboxMode: v.SandboxMode,
	}
}
