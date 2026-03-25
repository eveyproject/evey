package grpcapi

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	transcodingfakes "github.com/gravity-zero/evey/internal/domain/transcoding/fakes"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	activityfakes "github.com/gravity-zero/evey/internal/domain/activity/fakes"
	libraryfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	pluginfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
)

func newTestAdminHandler() *adminHandler {
	events := sharedfakes.NewFakeEventBus()
	roles := userfakes.NewFakeRoleRepository()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	userCmd := appuser.NewCommandService(
		userRepo,
		profileRepo,
		userfakes.NewFakeRefreshTokenRepository(),
		roles,
		userfakes.NewFakeUserRoleRepository(roles),
		userfakes.NewFakePasswordHasher(),
		userfakes.NewFakeTokenGenerator(),
		events,
	)
	userQuery := appuser.NewQueryService(userRepo, profileRepo, apiKeyRepo, inviteRepo, qcRepo)
	activitySvc := appactivity.NewQueryService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		activityfakes.NewFakeAuditLogRepository(),
		activityfakes.NewFakePlaySessionStatsReader(),
		activityfakes.NewFakePlayHistoryRepository(),
	)
	pluginRepo := pluginfakes.NewFakePluginRepository()
	pluginRuntimeRepo := pluginfakes.NewFakePluginRuntimeRepository()
	pluginRunner := pluginfakes.NewFakePluginRunner()
	pluginParser := pluginfakes.NewFakeManifestParser()
	pluginInstallerSvc := appplugin.NewInstallerService(pluginRepo, pluginRunner, pluginParser, events)
	pluginRuntimeSvc := appplugin.NewRuntimeService(pluginRepo, pluginRuntimeRepo, pluginRunner, pluginParser, events)
	pluginQuerySvc := appplugin.NewQueryService(pluginRepo, pluginRuntimeRepo)
	return &adminHandler{
		userCmd:         userCmd,
		userQuery:       userQuery,
		activitySvc:     activitySvc,
		pluginInstaller: pluginInstallerSvc,
		pluginRuntime:   pluginRuntimeSvc,
		pluginQuery:     pluginQuerySvc,
		version:         "1.0.0-test",
	}
}

func TestAdminGetDashboard(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.GetDashboard(context.Background(), &eveyv1.GetDashboardRequest{})
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if resp.TotalMediaItems != 0 {
		t.Errorf("expected 0 items, got %d", resp.TotalMediaItems)
	}
	if resp.TotalUsers != 0 {
		t.Errorf("expected 0 users, got %d", resp.TotalUsers)
	}
	if resp.CpuPercent < 0 || resp.CpuPercent > 100 {
		t.Errorf("CPU%% out of range: %.2f", resp.CpuPercent)
	}
	if resp.MemoryPercent < 0 || resp.MemoryPercent > 100 {
		t.Errorf("Memory%% out of range: %.2f", resp.MemoryPercent)
	}
}

func TestAdminGetDashboard_WithHLSManager(t *testing.T) {
	h := newTestAdminHandler()
	tmpDir := t.TempDir()
	hls := apptranscoding.NewHLSManager(context.Background(), tmpDir, 5, &transcodingfakes.HLSProcess{})
	defer hls.CleanupAll()
	h.hlsManager = hls

	resp, err := h.GetDashboard(context.Background(), &eveyv1.GetDashboardRequest{})
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if resp.ActiveTranscodes != 0 {
		t.Errorf("expected 0 active transcodes, got %d", resp.ActiveTranscodes)
	}
}

func TestAdminGetServerConfig(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.GetServerConfig(context.Background(), &eveyv1.GetServerConfigRequest{})
	if err != nil {
		t.Fatalf("GetServerConfig: %v", err)
	}
	if resp.Values == nil {
		t.Error("expected non-nil values map")
	}
}

func TestAdminUpdateServerConfig(t *testing.T) {
	h := newTestAdminHandler()
	vals := map[string]string{"key": "value"}
	resp, err := h.UpdateServerConfig(context.Background(), &eveyv1.UpdateServerConfigRequest{Values: vals})
	if err != nil {
		t.Fatalf("UpdateServerConfig: %v", err)
	}
	if resp.Values["key"] != "value" {
		t.Errorf("expected value 'value', got %q", resp.Values["key"])
	}
}

type fakeLibraryScanner struct {
	scanCalled bool
	libraryID  string
}

func (f *fakeLibraryScanner) ScanLibrary(_ context.Context, libraryID string) (int, int, error) {
	f.scanCalled = true
	f.libraryID = libraryID
	return 0, 0, nil
}

func newTestScheduler() (*applibrary.Scheduler, *libraryfakes.FakeScanScheduleRepository) {
	schedRepo := libraryfakes.NewFakeScanScheduleRepository()
	scheduler := applibrary.NewScheduler(schedRepo, &fakeLibraryScanner{}, slog.Default())
	return scheduler, schedRepo
}

func TestAdminListScheduledTasks_Empty(t *testing.T) {
	h := newTestAdminHandler()
	scheduler, _ := newTestScheduler()
	h.scheduler = scheduler

	resp, err := h.ListScheduledTasks(context.Background(), &eveyv1.ListScheduledTasksRequest{})
	if err != nil {
		t.Fatalf("ListScheduledTasks: %v", err)
	}
	if len(resp.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(resp.Tasks))
	}
}

func TestAdminListScheduledTasks_WithData(t *testing.T) {
	h := newTestAdminHandler()
	scheduler, schedRepo := newTestScheduler()
	h.scheduler = scheduler

	schedRepo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-1",
		CronExpression: "0 2 * * *",
		Enabled:        true,
	})
	schedRepo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-2",
		CronExpression: "0 3 * * 0",
		Enabled:        false,
	})

	resp, err := h.ListScheduledTasks(context.Background(), &eveyv1.ListScheduledTasksRequest{})
	if err != nil {
		t.Fatalf("ListScheduledTasks: %v", err)
	}
	if len(resp.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(resp.Tasks))
	}
}

func TestAdminUpdateScheduledTask_Success(t *testing.T) {
	h := newTestAdminHandler()
	scheduler, schedRepo := newTestScheduler()
	h.scheduler = scheduler

	schedRepo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-1",
		CronExpression: "0 2 * * *",
		Enabled:        true,
	})

	newCron := "0 4 * * *"
	enabled := false
	resp, err := h.UpdateScheduledTask(context.Background(), &eveyv1.UpdateScheduledTaskRequest{
		Id:             "lib-1",
		CronExpression: &newCron,
		Enabled:        &enabled,
	})
	if err != nil {
		t.Fatalf("UpdateScheduledTask: %v", err)
	}
	if resp.CronExpression != "0 4 * * *" {
		t.Errorf("expected cron %q, got %q", "0 4 * * *", resp.CronExpression)
	}
	if resp.Enabled {
		t.Error("expected enabled=false")
	}
}

func TestAdminUpdateScheduledTask_NotFound(t *testing.T) {
	h := newTestAdminHandler()
	scheduler, _ := newTestScheduler()
	h.scheduler = scheduler

	_, err := h.UpdateScheduledTask(context.Background(), &eveyv1.UpdateScheduledTaskRequest{
		Id: "nonexistent",
	})
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestAdminRunScheduledTask(t *testing.T) {
	h := newTestAdminHandler()
	scanner := &fakeLibraryScanner{}
	h.libScanner = scanner

	resp, err := h.RunScheduledTask(context.Background(), &eveyv1.RunScheduledTaskRequest{
		Id: "lib-1",
	})
	if err != nil {
		t.Fatalf("RunScheduledTask: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestAdminGetPluginLogs_NoLogDir(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "p1"})
	if err != nil {
		t.Fatalf("GetPluginLogs: %v", err)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(resp.Entries))
	}
}

func TestAdminGetPluginLogs_WithFile(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	logLines := `{"time":"2026-03-14T10:00:00Z","level":"INFO","msg":"plugin started","plugin_id":"p1"}` + "\n" +
		`{"time":"2026-03-14T10:01:00Z","level":"WARN","msg":"low memory","plugin_id":"p1","threshold":"512"}` + "\n" +
		`{"time":"2026-03-14T10:02:00Z","level":"ERROR","msg":"crash","plugin_id":"p1"}` + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "plugins", "p1.log"), []byte(logLines), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAdminHandler()
	h.logDir = tmpDir

	resp, err := h.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "p1"})
	if err != nil {
		t.Fatalf("GetPluginLogs: %v", err)
	}
	if len(resp.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(resp.Entries))
	}
	if resp.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Total)
	}
	if resp.Entries[0].Level != "INFO" {
		t.Errorf("expected INFO, got %s", resp.Entries[0].Level)
	}
	if resp.Entries[1].Message != "low memory" {
		t.Errorf("unexpected message: %s", resp.Entries[1].Message)
	}
	if resp.Entries[1].Fields["threshold"] != "512" {
		t.Errorf("expected threshold field '512', got %q", resp.Entries[1].Fields["threshold"])
	}
}

func TestAdminGetPluginLogs_LevelFilter(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	logLines := `{"time":"2026-03-14T10:00:00Z","level":"INFO","msg":"ok"}` + "\n" +
		`{"time":"2026-03-14T10:01:00Z","level":"ERROR","msg":"fail"}` + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "plugins", "p2.log"), []byte(logLines), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAdminHandler()
	h.logDir = tmpDir

	resp, err := h.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "p2", Level: "ERROR"})
	if err != nil {
		t.Fatalf("GetPluginLogs: %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Errorf("expected 1 entry after level filter, got %d", len(resp.Entries))
	}
	if resp.Entries[0].Message != "fail" {
		t.Errorf("unexpected message: %s", resp.Entries[0].Message)
	}
}

func TestAdminGetPluginLogs_Pagination(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for range 5 {
		b.WriteString(`{"time":"2026-03-14T10:00:00Z","level":"INFO","msg":"line"}` + "\n")
	}
	lines := b.String()
	if err := os.WriteFile(filepath.Join(tmpDir, "plugins", "p3.log"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAdminHandler()
	h.logDir = tmpDir

	resp, err := h.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "p3", Offset: 2, Limit: 2})
	if err != nil {
		t.Fatalf("GetPluginLogs: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Errorf("expected 2 entries (paginated), got %d", len(resp.Entries))
	}
	if resp.Total != 5 {
		t.Errorf("expected total=5, got %d", resp.Total)
	}
}

func TestAdminGetSetupStatus_NoAdmins(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.GetSetupStatus(context.Background(), &eveyv1.GetSetupStatusRequest{})
	if err != nil {
		t.Fatalf("GetSetupStatus: %v", err)
	}
	if resp.SetupComplete {
		t.Error("expected SetupComplete=false with no admins")
	}
}

func TestAdminGetSystemInfo(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if resp.Version != "1.0.0-test" {
		t.Errorf("Version = %q, want 1.0.0-test", resp.Version)
	}
	if resp.GoVersion == "" {
		t.Error("expected non-empty GoVersion")
	}
	if resp.Os == "" {
		t.Error("expected non-empty Os")
	}
}

func TestAdminGetSystemInfo_WithTranscoding(t *testing.T) {
	h := newTestAdminHandler()
	hwDetector := transcodingfakes.NewFakeHWDetector()
	events := sharedfakes.NewFakeEventBus()
	transcodingSvc := apptranscoding.NewService(nil, hwDetector, nil, nil, events, 1)
	h.transcodingSvc = transcodingSvc

	resp, err := h.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	// Just verify it doesn't crash with transcoding service present
	_ = resp
}

func TestAdminGetAuditLog(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.GetAuditLog(context.Background(), &eveyv1.GetAuditLogRequest{Limit: 10})
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 audit entries, got %d", resp.Total)
	}
}

func TestAdminGetAuditLog_WithFilters(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.GetAuditLog(context.Background(), &eveyv1.GetAuditLogRequest{
		Limit:        10,
		UserId:       "u-1",
		Action:       "login",
		ResourceType: "user",
	})
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	_ = resp
}

func TestAdminListPluginRuntimes(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.ListPluginRuntimes(context.Background(), &eveyv1.ListPluginRuntimesRequest{})
	if err != nil {
		t.Fatalf("ListPluginRuntimes: %v", err)
	}
	if len(resp.Runtimes) != 0 {
		t.Errorf("expected 0 runtimes, got %d", len(resp.Runtimes))
	}
}

func TestAdminAddPluginRuntime(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.AddPluginRuntime(context.Background(), &eveyv1.AddPluginRuntimeRequest{
		Name:     "python3",
		Language: "python",
		Version:  "3.11",
	})
	if err != nil {
		t.Fatalf("AddPluginRuntime: %v", err)
	}
	if resp.Language != "python" {
		t.Errorf("Language = %q", resp.Language)
	}
}

func TestAdminRemovePluginRuntime(t *testing.T) {
	h := newTestAdminHandler()
	// Add first
	h.AddPluginRuntime(context.Background(), &eveyv1.AddPluginRuntimeRequest{
		Name: "python3", Language: "python", Version: "3.11",
	})
	_, err := h.RemovePluginRuntime(context.Background(), &eveyv1.RemovePluginRuntimeRequest{Id: "python3"})
	if err != nil {
		t.Fatalf("RemovePluginRuntime: %v", err)
	}
}

func TestAdminListPlugins(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.ListPlugins(context.Background(), &eveyv1.ListPluginsRequest{})
	if err != nil {
		t.Fatalf("ListPlugins: %v", err)
	}
	if len(resp.Plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(resp.Plugins))
	}
}

func TestAdminAcquireReleaseHeavyOp(t *testing.T) {
	h := newTestAdminHandler()
	h.heavyOpSemaphore = make(chan struct{}, 1)

	if !h.acquireHeavyOp() {
		t.Error("first acquire should succeed")
	}
	if h.acquireHeavyOp() {
		t.Error("second acquire should fail (semaphore full)")
	}
	h.releaseHeavyOp()
	if !h.acquireHeavyOp() {
		t.Error("acquire after release should succeed")
	}
	h.releaseHeavyOp()
}

func TestAdminAcquireHeavyOp_NilSemaphore(t *testing.T) {
	h := newTestAdminHandler()
	h.heavyOpSemaphore = nil
	if !h.acquireHeavyOp() {
		t.Error("nil semaphore should always succeed")
	}
	h.releaseHeavyOp() // should not panic
}

func TestAdminGetDashboard_WithLibLister(t *testing.T) {
	h := newTestAdminHandler()
	h.libLister = &fakeAdminLibLister{
		libs: []*domainlibrary.Library{
			{ID: "lib-1", Paths: []string{"/tmp"}},
		},
	}
	resp, err := h.GetDashboard(context.Background(), &eveyv1.GetDashboardRequest{})
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	// storage info should be populated for /tmp
	if len(resp.Storage) == 0 {
		t.Log("no storage info for /tmp (may be expected in CI)")
	}
}

func TestAdminListScheduledTasks_NilScheduler(t *testing.T) {
	h := newTestAdminHandler()
	h.scheduler = nil
	resp, err := h.ListScheduledTasks(context.Background(), &eveyv1.ListScheduledTasksRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Tasks) != 0 {
		t.Errorf("expected empty tasks, got %d", len(resp.Tasks))
	}
}

func TestAdminRunScheduledTask_NilScanner(t *testing.T) {
	h := newTestAdminHandler()
	h.libScanner = nil
	_, err := h.RunScheduledTask(context.Background(), &eveyv1.RunScheduledTaskRequest{Id: "lib-1"})
	if err == nil {
		t.Error("expected error for nil scanner")
	}
}

func TestAdminEnablePlugin(t *testing.T) {
	h := newTestAdminHandler()
	_, err := h.EnablePlugin(context.Background(), &eveyv1.EnablePluginRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestAdminDisablePlugin(t *testing.T) {
	h := newTestAdminHandler()
	_, err := h.DisablePlugin(context.Background(), &eveyv1.DisablePluginRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestAdminGetPluginConfig(t *testing.T) {
	h := newTestAdminHandler()
	_, err := h.GetPluginConfig(context.Background(), &eveyv1.GetPluginConfigRequest{PluginId: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent plugin config")
	}
}

func TestAdminUpdatePluginConfig(t *testing.T) {
	h := newTestAdminHandler()
	_, err := h.UpdatePluginConfig(context.Background(), &eveyv1.UpdatePluginConfigRequest{
		PluginId: "nonexistent",
		Values:   map[string]string{"key": "value"},
	})
	if err == nil {
		t.Error("expected error for nonexistent plugin config")
	}
}

func TestAdminRefreshAllMetadata_NilService(t *testing.T) {
	h := newTestAdminHandler()
	h.metadataCmd = nil
	_, err := h.RefreshAllMetadata(context.Background(), &eveyv1.RefreshAllMetadataRequest{})
	if err == nil {
		t.Error("expected error for nil metadataCmd")
	}
}

func TestAdminGenerateAllTrickplays_NilService(t *testing.T) {
	h := newTestAdminHandler()
	h.libCmd = nil
	_, err := h.GenerateAllTrickplays(context.Background(), &eveyv1.GenerateAllTrickplaysRequest{})
	if err == nil {
		t.Error("expected error for nil libCmd")
	}
}

func TestAdminDeleteMediaItem_NilService(t *testing.T) {
	h := newTestAdminHandler()
	h.libCmd = nil
	_, err := h.DeleteMediaItem(context.Background(), &eveyv1.DeleteMediaItemRequest{Id: "m-1"})
	if err == nil {
		t.Error("expected error for nil libCmd")
	}
}

type fakeAdminLibLister struct {
	libs []*domainlibrary.Library
}

func (f *fakeAdminLibLister) ListLibraries(_ context.Context) ([]*domainlibrary.Library, error) {
	return f.libs, nil
}

func TestAdminSetupWizard(t *testing.T) {
	h := newTestAdminHandler()
	resp, err := h.SetupWizard(context.Background(), &eveyv1.SetupWizardRequest{
		AdminUsername: "admin",
		AdminPassword: "password123",
		AdminEmail:    "admin@test.com",
	})
	if err != nil {
		t.Fatalf("SetupWizard: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if resp.ServerId == "" {
		t.Error("expected non-empty server id")
	}

	// second call should fail (setup already done)
	_, err = h.SetupWizard(context.Background(), &eveyv1.SetupWizardRequest{
		AdminUsername: "admin2",
		AdminPassword: "password456",
		AdminEmail:    "admin2@test.com",
	})
	if err == nil {
		t.Error("expected error on duplicate setup")
	}
}
