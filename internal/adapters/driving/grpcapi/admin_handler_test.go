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
		apiKeyRepo,
		inviteRepo,
		qcRepo,
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
	pluginRuntimeSvc := appplugin.NewRuntimeService(pluginRepo, pluginRunner, pluginParser, events)
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
	hls := apptranscoding.NewHLSManager(tmpDir, 5)
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
