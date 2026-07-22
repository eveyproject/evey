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

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appplugin "github.com/evey-project/evey/internal/app/plugin"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
}

// requireEnabled gates state-changing or executing plugin operations.
// Read-only methods (List*, Get*) intentionally bypass this so the
// admin UI can render a consistent "plugins disabled" state without
// 500-ing.
func (d *adminPluginHandler) requireEnabled() error {
	if !d.enabled {
		return status.Error(codes.FailedPrecondition,
			"plugin system is disabled (set EVEY_PLUGINS_ENABLED=true to enable)")
	}
	return nil
}

func (d *adminPluginHandler) IssuePluginToken(ctx context.Context, req *eveyv1.IssuePluginTokenRequest) (*eveyv1.IssuePluginTokenResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.IssuePluginToken")
	defer span.End()
	if err := d.requireEnabled(); err != nil {
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
		return nil, status.Error(codes.InvalidArgument, "plugin_id required")
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
			return nil, status.Error(codes.NotFound, "plugin not found")
		case errors.Is(err, appplugin.ErrPluginNotEnabled):
			span.SetStatus(otelcodes.Error, "plugin not enabled")
			return nil, status.Error(codes.FailedPrecondition, "plugin not enabled")
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
	if err := d.requireEnabled(); err != nil {
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
	if err := d.requireEnabled(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := d.pluginRuntime.RemoveRuntime(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "remove runtime", err)
	}
	return &eveyv1.RemovePluginRuntimeResponse{}, nil
}

func (d *adminPluginHandler) InstallPlugin(ctx context.Context, req *eveyv1.InstallPluginRequest) (*eveyv1.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.InstallPlugin")
	defer span.End()
	if err := d.requireEnabled(); err != nil {
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
		return nil, internalError(ctx, "install plugin", err)
	}
	return pluginToProto(p), nil
}

func (d *adminPluginHandler) UninstallPlugin(ctx context.Context, req *eveyv1.UninstallPluginRequest) (*eveyv1.UninstallPluginResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.UninstallPlugin")
	defer span.End()
	if err := d.requireEnabled(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := d.pluginInstaller.UninstallPlugin(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "uninstall plugin", err)
	}
	return &eveyv1.UninstallPluginResponse{}, nil
}

func (d *adminPluginHandler) EnablePlugin(ctx context.Context, req *eveyv1.EnablePluginRequest) (*eveyv1.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.EnablePlugin")
	defer span.End()
	if err := d.requireEnabled(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := d.pluginRuntime.EnablePlugin(ctx, req.Id); err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appplugin.ErrPluginNotFound):
			span.SetStatus(otelcodes.Error, "plugin not found")
			return nil, status.Error(codes.NotFound, "plugin not found")
		case errors.Is(err, appplugin.ErrAlreadyRunning):
			span.SetStatus(otelcodes.Error, "plugin already running")
			return nil, status.Error(codes.FailedPrecondition, "plugin already running")
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
	if err := d.requireEnabled(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := d.pluginRuntime.DisablePlugin(ctx, req.Id); err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appplugin.ErrPluginNotFound):
			span.SetStatus(otelcodes.Error, "plugin not found")
			return nil, status.Error(codes.NotFound, "plugin not found")
		case errors.Is(err, appplugin.ErrNotRunning):
			span.SetStatus(otelcodes.Error, "plugin not running")
			return nil, status.Error(codes.FailedPrecondition, "plugin not running")
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
		resp.Plugins = append(resp.Plugins, pluginToProto(p))
	}
	return resp, nil
}

func (d *adminPluginHandler) GetPluginConfig(ctx context.Context, req *eveyv1.GetPluginConfigRequest) (*eveyv1.PluginConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetPluginConfig")
	defer span.End()
	result, err := d.pluginQuery.GetPluginConfig(ctx, req.PluginId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "get config")
		return nil, status.Errorf(codes.NotFound, "get config: %v", err)
	}
	values := make(map[string]string)
	for k, v := range result.Config {
		values[k] = stringFromInterface(v)
	}
	resp := &eveyv1.PluginConfig{
		PluginId: req.PluginId,
		Values:   values,
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
	if err := d.requireEnabled(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	result, err := d.pluginQuery.GetPluginConfig(ctx, req.PluginId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "get plugin")
		return nil, status.Errorf(codes.NotFound, "get plugin: %v", err)
	}
	if err := validateConfigValues(req.Values, result.ConfigSchema); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	cfg := coerceConfigValues(req.Values, result.ConfigSchema)
	if err := d.pluginRuntime.UpdatePluginConfig(ctx, req.PluginId, cfg); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "update config", err)
	}
	return &eveyv1.PluginConfig{
		PluginId: req.PluginId,
		Values:   req.Values,
	}, nil
}

func (d *adminPluginHandler) PreviewPluginUpdate(ctx context.Context, req *eveyv1.PreviewPluginUpdateRequest) (*eveyv1.PreviewPluginUpdateResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.PreviewPluginUpdate")
	defer span.End()
	if err := d.requireEnabled(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if req.PluginId == "" {
		return nil, status.Error(codes.InvalidArgument, "plugin_id required")
	}
	if req.Source == "" {
		return nil, status.Error(codes.InvalidArgument, "source required")
	}
	preview, err := d.pluginInstaller.PreviewUpdate(ctx, req.PluginId, req.Source)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		if errors.Is(err, appplugin.ErrPluginNotFound) {
			return nil, status.Error(codes.NotFound, "plugin not found")
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
	if err := d.requireEnabled(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if req.PluginId == "" {
		return nil, status.Error(codes.InvalidArgument, "plugin_id required")
	}
	if req.Source == "" {
		return nil, status.Error(codes.InvalidArgument, "source required")
	}
	if req.AcknowledgedHash == "" {
		return nil, status.Error(codes.InvalidArgument, "acknowledged_hash required")
	}
	p, err := d.pluginInstaller.ApplyUpdate(ctx, req.PluginId, req.Source, req.AcknowledgedHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case errors.Is(err, appplugin.ErrPluginNotFound):
			return nil, status.Error(codes.NotFound, "plugin not found")
		case errors.Is(err, appplugin.ErrCandidateChanged):
			return nil, status.Error(codes.Aborted, err.Error())
		default:
			return nil, internalError(ctx, "apply update", err)
		}
	}
	return pluginToProto(p), nil
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
		return nil, status.Error(codes.InvalidArgument, "invalid plugin id")
	}
	logPath := filepath.Join(d.logDir, "plugins", req.PluginId+".log")
	f, err := os.Open(logPath) //nolint:gosec
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &eveyv1.GetPluginLogsResponse{}, nil
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "open log", err)
	}
	defer f.Close()

	filterLevel := strings.ToUpper(req.Level)
	var all []*eveyv1.PluginLogEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
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
		if filterLevel != "" && !strings.EqualFold(level, filterLevel) {
			continue
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
		entry := &eveyv1.PluginLogEntry{
			Level:   level,
			Message: msg,
			Fields:  fields,
		}
		if !ts.IsZero() {
			entry.Timestamp = timestamppb.New(ts)
		}
		all = append(all, entry)
	}

	total := int32(len(all))
	offset := max(int(req.Offset), 0)
	limit := int(req.Limit)
	if offset >= len(all) {
		return &eveyv1.GetPluginLogsResponse{Total: total}, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return &eveyv1.GetPluginLogsResponse{Entries: all, Total: total}, nil
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
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if req.MemoryMb < 0 || req.PidsMax < 0 || req.DiskMaxMb < 0 {
		return nil, status.Error(codes.InvalidArgument, "memory_mb, pids_max and disk_max_mb must be >= 0")
	}
	if req.CpuNice < 0 || req.CpuNice > 19 {
		return nil, status.Error(codes.InvalidArgument, "cpu_nice must be between 0 and 19")
	}
	if req.SandboxMode != "" && !appplugin.IsValidSandboxMode(req.SandboxMode) {
		return nil, status.Error(codes.InvalidArgument, "sandbox_mode must be one of: auto, landlock, bubblewrap, strict, basic")
	}
	if d.serverCfg == nil {
		return nil, status.Error(codes.FailedPrecondition, "server config store unavailable")
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
