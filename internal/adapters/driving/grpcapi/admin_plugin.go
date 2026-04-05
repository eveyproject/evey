package grpcapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	"github.com/gravity-zero/evey/internal/domain/plugin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type adminPluginHandler struct {
	eveyv1.UnimplementedAdminPluginServiceServer
	pluginInstaller *appplugin.InstallerService
	pluginRuntime   *appplugin.RuntimeService
	pluginQuery     *appplugin.QueryService
	logDir          string
}

func (d *adminPluginHandler) ListPluginRuntimes(ctx context.Context, _ *eveyv1.ListPluginRuntimesRequest) (*eveyv1.ListPluginRuntimesResponse, error) {
	runtimes, err := d.pluginQuery.ListRuntimes(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list runtimes: %v", err)
	}
	resp := &eveyv1.ListPluginRuntimesResponse{}
	for _, rt := range runtimes {
		resp.Runtimes = append(resp.Runtimes, pluginRuntimeToProto(rt))
	}
	return resp, nil
}

func (d *adminPluginHandler) AddPluginRuntime(ctx context.Context, req *eveyv1.AddPluginRuntimeRequest) (*eveyv1.PluginRuntime, error) {
	rt := &plugin.PluginRuntime{
		ID:       req.Name,
		Language: req.Language,
		Version:  req.Version,
	}
	if err := d.pluginRuntime.AddRuntime(ctx, rt); err != nil {
		return nil, status.Errorf(codes.Internal, "add runtime: %v", err)
	}
	return pluginRuntimeToProto(rt), nil
}

func (d *adminPluginHandler) RemovePluginRuntime(ctx context.Context, req *eveyv1.RemovePluginRuntimeRequest) (*eveyv1.RemovePluginRuntimeResponse, error) {
	if err := d.pluginRuntime.RemoveRuntime(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "remove runtime: %v", err)
	}
	return &eveyv1.RemovePluginRuntimeResponse{}, nil
}

func (d *adminPluginHandler) InstallPlugin(ctx context.Context, req *eveyv1.InstallPluginRequest) (*eveyv1.Plugin, error) {
	p, err := d.pluginInstaller.InstallPlugin(ctx, req.Source)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "install plugin: %v", err)
	}
	return pluginToProto(p), nil
}

func (d *adminPluginHandler) UninstallPlugin(ctx context.Context, req *eveyv1.UninstallPluginRequest) (*eveyv1.UninstallPluginResponse, error) {
	if err := d.pluginInstaller.UninstallPlugin(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "uninstall plugin: %v", err)
	}
	return &eveyv1.UninstallPluginResponse{}, nil
}

func (d *adminPluginHandler) EnablePlugin(ctx context.Context, req *eveyv1.EnablePluginRequest) (*eveyv1.Plugin, error) {
	if err := d.pluginRuntime.EnablePlugin(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "enable plugin: %v", err)
	}
	return &eveyv1.Plugin{Id: req.Id, Enabled: true}, nil
}

func (d *adminPluginHandler) DisablePlugin(ctx context.Context, req *eveyv1.DisablePluginRequest) (*eveyv1.Plugin, error) {
	if err := d.pluginRuntime.DisablePlugin(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "disable plugin: %v", err)
	}
	return &eveyv1.Plugin{Id: req.Id, Enabled: false}, nil
}

func (d *adminPluginHandler) ListPlugins(ctx context.Context, _ *eveyv1.ListPluginsRequest) (*eveyv1.ListPluginsResponse, error) {
	plugins, err := d.pluginQuery.ListPlugins(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list plugins: %v", err)
	}
	resp := &eveyv1.ListPluginsResponse{}
	for _, p := range plugins {
		resp.Plugins = append(resp.Plugins, pluginToProto(p))
	}
	return resp, nil
}

func (d *adminPluginHandler) GetPluginConfig(ctx context.Context, req *eveyv1.GetPluginConfigRequest) (*eveyv1.PluginConfig, error) {
	result, err := d.pluginQuery.GetPluginConfig(ctx, req.PluginId)
	if err != nil {
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
	result, err := d.pluginQuery.GetPluginConfig(ctx, req.PluginId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "get plugin: %v", err)
	}
	if err := validateConfigValues(req.Values, result.ConfigSchema); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	cfg := coerceConfigValues(req.Values, result.ConfigSchema)
	if err := d.pluginRuntime.UpdatePluginConfig(ctx, req.PluginId, cfg); err != nil {
		return nil, status.Errorf(codes.Internal, "update config: %v", err)
	}
	return &eveyv1.PluginConfig{
		PluginId: req.PluginId,
		Values:   req.Values,
	}, nil
}

func (d *adminPluginHandler) GetPluginLogs(_ context.Context, req *eveyv1.GetPluginLogsRequest) (*eveyv1.GetPluginLogsResponse, error) {
	if d.logDir == "" || req.PluginId == "" {
		return &eveyv1.GetPluginLogsResponse{}, nil
	}
	logPath := filepath.Join(d.logDir, "plugins", req.PluginId+".log")
	f, err := os.Open(logPath) //nolint:gosec
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &eveyv1.GetPluginLogsResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "open log: %v", err)
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
	offset := int(req.Offset)
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
