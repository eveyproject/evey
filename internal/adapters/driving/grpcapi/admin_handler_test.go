package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	activityfakes "github.com/gravity-zero/evey/internal/domain/activity/fakes"
	pluginfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
)

func newTestAdminHandler() *adminHandler {
	events := sharedfakes.NewFakeEventBus()
	roles := userfakes.NewFakeRoleRepository()
	userSvc := appuser.NewService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeProfileRepository(),
		userfakes.NewFakeRefreshTokenRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		roles,
		userfakes.NewFakeUserRoleRepository(roles),
		userfakes.NewFakePasswordHasher(),
		userfakes.NewFakeTokenGenerator(),
		events,
	)
	activitySvc := appactivity.NewQueryService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		activityfakes.NewFakeAuditLogRepository(),
		activityfakes.NewFakePlaySessionStatsReader(),
		activityfakes.NewFakePlayHistoryRepository(),
	)
	pluginSvc := appplugin.NewService(
		pluginfakes.NewFakePluginRepository(),
		pluginfakes.NewFakePluginRuntimeRepository(),
		pluginfakes.NewFakePluginRunner(),
		pluginfakes.NewFakeManifestParser(),
		events,
	)
	return &adminHandler{
		userSvc:     userSvc,
		activitySvc: activitySvc,
		pluginSvc:   pluginSvc,
		version:     "1.0.0-test",
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
