package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"runtime"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/domain/activity"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/pkg/sysinfo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type adminLibraryLister interface {
	ListLibraries(ctx context.Context) ([]*domainlibrary.Library, error)
}

type adminScheduler interface {
	ListSchedules(ctx context.Context) ([]*domainlibrary.ScanSchedule, error)
	SetSchedule(ctx context.Context, libraryID, cronExpr string, enabled bool) (*domainlibrary.ScanSchedule, error)
}

type adminLibraryScanner interface {
	ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error)
}

type adminHandler struct {
	eveyv1.UnimplementedAdminServiceServer
	userCmd        *appuser.CommandService
	userQuery      *appuser.QueryService
	transcodingSvc *apptranscoding.Service
	activitySvc    *appactivity.QueryService
	pluginSvc      *appplugin.Service
	hlsManager     *apptranscoding.HLSManager
	libLister      adminLibraryLister
	scheduler      adminScheduler
	libScanner     adminLibraryScanner
	version        string
}

func (h *adminHandler) GetSystemInfo(ctx context.Context, _ *eveyv1.GetSystemInfoRequest) (*eveyv1.SystemInfo, error) {
	admins, adminErr := h.userQuery.CountAdmins(ctx)
	if adminErr != nil {
		slog.Warn("count admins", slog.Any("err", adminErr))
	}
	return &eveyv1.SystemInfo{
		Version:       h.version,
		Os:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		GoVersion:     runtime.Version(),
		SetupComplete: admins > 0,
	}, nil
}

func (h *adminHandler) GetServerConfig(_ context.Context, _ *eveyv1.GetServerConfigRequest) (*eveyv1.ServerConfig, error) {
	return &eveyv1.ServerConfig{Values: map[string]string{}}, nil
}

func (h *adminHandler) UpdateServerConfig(_ context.Context, req *eveyv1.UpdateServerConfigRequest) (*eveyv1.ServerConfig, error) {
	return &eveyv1.ServerConfig{Values: req.Values}, nil
}

func (h *adminHandler) SetupWizard(ctx context.Context, req *eveyv1.SetupWizardRequest) (*eveyv1.SetupWizardResponse, error) {
	u, err := h.userCmd.Setup(ctx, req.AdminUsername, req.AdminPassword, req.AdminEmail)
	if err != nil {
		if errors.Is(err, appuser.ErrSetupDone) {
			return nil, status.Errorf(codes.AlreadyExists, "setup already completed")
		}
		return nil, status.Errorf(codes.Internal, "setup: %v", err)
	}
	access, refresh, err := h.userCmd.Login(ctx, req.AdminUsername, req.AdminPassword)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "login after setup: %v", err)
	}
	return &eveyv1.SetupWizardResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ServerId:     u.ID,
	}, nil
}

func (h *adminHandler) GetDashboard(ctx context.Context, _ *eveyv1.GetDashboardRequest) (*eveyv1.DashboardInfo, error) {
	stats, err := h.activitySvc.GetServerStats(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get server stats: %v", err)
	}

	sys := sysinfo.Collect()

	activeTranscodes := int32(stats.ActiveTranscodes)
	if h.hlsManager != nil {
		hlsCount := h.hlsManager.ActiveSessionCount()
		if int32(hlsCount) > activeTranscodes {
			activeTranscodes = int32(hlsCount)
		}
	}

	resp := &eveyv1.DashboardInfo{
		ActiveStreams:    int32(stats.ActiveStreams),
		ActiveTranscodes: activeTranscodes,
		TotalMediaItems:  int32(stats.TotalItems),
		TotalUsers:       int32(stats.TotalUsers),
		CpuPercent:       sys.CPUPercent,
		MemoryPercent:    sys.MemoryPercent,
	}

	if h.libLister != nil {
		libs, err := h.libLister.ListLibraries(ctx)
		if err == nil {
			var paths []string
			for _, lib := range libs {
				paths = append(paths, lib.Paths...)
			}
			for _, st := range sysinfo.GetStorageStats(paths) {
				resp.Storage = append(resp.Storage, &eveyv1.StorageInfo{
					Path:       st.Path,
					TotalBytes: st.TotalBytes,
					UsedBytes:  st.UsedBytes,
					FreeBytes:  st.FreeBytes,
				})
			}
		}
	}

	return resp, nil
}

func (h *adminHandler) ListScheduledTasks(ctx context.Context, _ *eveyv1.ListScheduledTasksRequest) (*eveyv1.ListScheduledTasksResponse, error) {
	if h.scheduler == nil {
		return &eveyv1.ListScheduledTasksResponse{}, nil
	}
	schedules, err := h.scheduler.ListSchedules(ctx)
	if err != nil {
		slog.Error("list scheduled tasks", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListScheduledTasksResponse{}
	for _, s := range schedules {
		task := &eveyv1.ScheduledTask{
			Id:             s.LibraryID,
			Name:           "Library scan: " + s.LibraryID,
			CronExpression: s.CronExpression,
			Enabled:        s.Enabled,
		}
		if s.LastRunAt != nil {
			task.LastRunAt = timestamppb.New(*s.LastRunAt)
		}
		if s.NextRunAt != nil {
			task.NextRunAt = timestamppb.New(*s.NextRunAt)
		}
		resp.Tasks = append(resp.Tasks, task)
	}
	return resp, nil
}

func (h *adminHandler) UpdateScheduledTask(ctx context.Context, req *eveyv1.UpdateScheduledTaskRequest) (*eveyv1.ScheduledTask, error) {
	if h.scheduler == nil {
		return nil, status.Errorf(codes.Unimplemented, "scheduler not available")
	}
	existing, err := h.scheduler.ListSchedules(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	var current *domainlibrary.ScanSchedule
	for _, s := range existing {
		if s.LibraryID == req.Id {
			current = s
			break
		}
	}
	if current == nil {
		return nil, status.Errorf(codes.NotFound, "scheduled task not found")
	}
	cron := current.CronExpression
	if req.CronExpression != nil {
		cron = *req.CronExpression
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	updated, err := h.scheduler.SetSchedule(ctx, req.Id, cron, enabled)
	if err != nil {
		slog.Error("update scheduled task", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	task := &eveyv1.ScheduledTask{
		Id:             updated.LibraryID,
		Name:           "Library scan: " + updated.LibraryID,
		CronExpression: updated.CronExpression,
		Enabled:        updated.Enabled,
	}
	if updated.NextRunAt != nil {
		task.NextRunAt = timestamppb.New(*updated.NextRunAt)
	}
	return task, nil
}

func (h *adminHandler) RunScheduledTask(ctx context.Context, req *eveyv1.RunScheduledTaskRequest) (*eveyv1.RunScheduledTaskResponse, error) {
	if h.libScanner == nil {
		return nil, status.Errorf(codes.Unimplemented, "scanner not available")
	}
	go func() {
		added, updated, err := h.libScanner.ScanLibrary(context.Background(), req.Id)
		if err != nil {
			slog.Error("manual scan failed", slog.String("libraryID", req.Id), slog.Any("err", err))
		} else {
			slog.Info("manual scan done", slog.String("libraryID", req.Id), slog.Int("added", added), slog.Int("updated", updated))
		}
	}()
	return &eveyv1.RunScheduledTaskResponse{}, nil
}

func (h *adminHandler) GetAuditLog(ctx context.Context, req *eveyv1.GetAuditLogRequest) (*eveyv1.GetAuditLogResponse, error) {
	filter := activity.AuditLogFilter{
		Limit:  int(req.Limit),
		Offset: int(req.Offset),
	}
	if req.UserId != "" {
		filter.UserID = &req.UserId
	}
	if req.Action != "" {
		filter.Action = &req.Action
	}
	if req.ResourceType != "" {
		filter.ResourceType = &req.ResourceType
	}
	entries, err := h.activitySvc.GetAuditLog(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get audit log: %v", err)
	}
	resp := &eveyv1.GetAuditLogResponse{Total: int32(len(entries))}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, auditLogEntryToProto(e))
	}
	return resp, nil
}

func (h *adminHandler) ListPluginRuntimes(ctx context.Context, _ *eveyv1.ListPluginRuntimesRequest) (*eveyv1.ListPluginRuntimesResponse, error) {
	runtimes, err := h.pluginSvc.ListRuntimes(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list runtimes: %v", err)
	}
	resp := &eveyv1.ListPluginRuntimesResponse{}
	for _, rt := range runtimes {
		resp.Runtimes = append(resp.Runtimes, pluginRuntimeToProto(rt))
	}
	return resp, nil
}

func (h *adminHandler) AddPluginRuntime(ctx context.Context, req *eveyv1.AddPluginRuntimeRequest) (*eveyv1.PluginRuntime, error) {
	rt := &plugin.PluginRuntime{
		ID:       req.Name,
		Language: req.Language,
		Version:  req.Version,
	}
	if err := h.pluginSvc.AddRuntime(ctx, rt); err != nil {
		return nil, status.Errorf(codes.Internal, "add runtime: %v", err)
	}
	return pluginRuntimeToProto(rt), nil
}

func (h *adminHandler) RemovePluginRuntime(ctx context.Context, req *eveyv1.RemovePluginRuntimeRequest) (*eveyv1.RemovePluginRuntimeResponse, error) {
	if err := h.pluginSvc.RemoveRuntime(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "remove runtime: %v", err)
	}
	return &eveyv1.RemovePluginRuntimeResponse{}, nil
}

func (h *adminHandler) InstallPlugin(ctx context.Context, req *eveyv1.InstallPluginRequest) (*eveyv1.Plugin, error) {
	p, err := h.pluginSvc.InstallPlugin(ctx, req.Source)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "install plugin: %v", err)
	}
	return pluginToProto(p), nil
}

func (h *adminHandler) UninstallPlugin(ctx context.Context, req *eveyv1.UninstallPluginRequest) (*eveyv1.UninstallPluginResponse, error) {
	if err := h.pluginSvc.UninstallPlugin(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "uninstall plugin: %v", err)
	}
	return &eveyv1.UninstallPluginResponse{}, nil
}

func (h *adminHandler) EnablePlugin(ctx context.Context, req *eveyv1.EnablePluginRequest) (*eveyv1.Plugin, error) {
	if err := h.pluginSvc.EnablePlugin(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "enable plugin: %v", err)
	}
	return &eveyv1.Plugin{Id: req.Id, Enabled: true}, nil
}

func (h *adminHandler) DisablePlugin(ctx context.Context, req *eveyv1.DisablePluginRequest) (*eveyv1.Plugin, error) {
	if err := h.pluginSvc.DisablePlugin(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "disable plugin: %v", err)
	}
	return &eveyv1.Plugin{Id: req.Id, Enabled: false}, nil
}

func (h *adminHandler) ListPlugins(ctx context.Context, _ *eveyv1.ListPluginsRequest) (*eveyv1.ListPluginsResponse, error) {
	plugins, err := h.pluginSvc.ListPlugins(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list plugins: %v", err)
	}
	resp := &eveyv1.ListPluginsResponse{}
	for _, p := range plugins {
		resp.Plugins = append(resp.Plugins, pluginToProto(p))
	}
	return resp, nil
}

func (h *adminHandler) GetPluginConfig(ctx context.Context, req *eveyv1.GetPluginConfigRequest) (*eveyv1.PluginConfig, error) {
	cfg, err := h.pluginSvc.GetPluginConfig(ctx, req.PluginId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "get config: %v", err)
	}
	values := make(map[string]string)
	for k, v := range cfg {
		values[k] = stringFromInterface(v)
	}
	return &eveyv1.PluginConfig{
		PluginId: req.PluginId,
		Values:   values,
	}, nil
}

func (h *adminHandler) UpdatePluginConfig(ctx context.Context, req *eveyv1.UpdatePluginConfigRequest) (*eveyv1.PluginConfig, error) {
	cfg := make(map[string]any)
	for k, v := range req.Values {
		cfg[k] = v
	}
	if err := h.pluginSvc.UpdatePluginConfig(ctx, req.PluginId, cfg); err != nil {
		return nil, status.Errorf(codes.Internal, "update config: %v", err)
	}
	return &eveyv1.PluginConfig{
		PluginId: req.PluginId,
		Values:   req.Values,
	}, nil
}

func (h *adminHandler) GetPluginLogs(_ context.Context, req *eveyv1.GetPluginLogsRequest) (*eveyv1.GetPluginLogsResponse, error) {
	_ = req.PluginId
	return &eveyv1.GetPluginLogsResponse{}, nil
}

func stringFromInterface(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
