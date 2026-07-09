package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appconfig "github.com/evey-project/evey/internal/app/config"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	transcodingfakes "github.com/evey-project/evey/internal/domain/transcoding/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAdminGetSetupStatus_NoAdmins(t *testing.T) {
	d := newTestSystemHandler()
	resp, err := d.GetSetupStatus(context.Background(), &eveyv1.GetSetupStatusRequest{})
	if err != nil {
		t.Fatalf("GetSetupStatus: %v", err)
	}
	if resp.SetupComplete {
		t.Error("expected SetupComplete=false with no admins")
	}
}

// The public (unauthenticated) setup status must surface password recovery so
// the logged-out login page can show the "Forgot password?" link.
func TestAdminGetSetupStatus_ExposesPasswordRecovery(t *testing.T) {
	d := newTestSystemHandler()
	events := sharedfakes.NewFakeEventBus()
	roles := userfakes.NewFakeRoleRepository()
	serverCfg := sharedfakes.NewFakeServerConfigRepository()
	if err := serverCfg.Set(context.Background(), "auth.password_recovery_enabled", "true"); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	d.userCmd = appuser.NewCommandService(
		userfakes.NewFakeUserRepository(), userfakes.NewFakeProfileRepository(),
		userfakes.NewFakeRefreshTokenRepository(), roles, userfakes.NewFakeUserRoleRepository(roles),
		userfakes.NewFakePasswordHasher(), userfakes.NewFakeTokenGenerator(), events,
		7*24*time.Hour, userfakes.NewFakeQuickConnectRepository(),
		appuser.WithPasswordReset(userfakes.NewFakePasswordResetTokenRepository(), serverCfg, true),
	)

	resp, err := d.GetSetupStatus(context.Background(), &eveyv1.GetSetupStatusRequest{})
	if err != nil {
		t.Fatalf("GetSetupStatus: %v", err)
	}
	if !resp.PasswordRecoveryEnabled {
		t.Error("expected public setup status to expose PasswordRecoveryEnabled=true")
	}
}

func TestAdminGetSystemInfo(t *testing.T) {
	d := newTestSystemHandler()
	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
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
	d := newTestSystemHandler()
	hwDetector := transcodingfakes.NewFakeHWDetector()
	events := sharedfakes.NewFakeEventBus()
	d.transcodingSvc = apptranscoding.NewService(nil, hwDetector, nil, nil, events, 1)

	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	_ = resp
}

// fakeCalibrationSnapshot satisfies calibrationSnapshotProvider for tests.
type fakeCalibrationSnapshot struct {
	capability domaintranscoding.EncodeCapability
}

func (f fakeCalibrationSnapshot) Current() domaintranscoding.EncodeCapability { return f.capability }

// fakeVideoActiveReporter satisfies videoSessionActiveReporter for tests.
type fakeVideoActiveReporter struct{ active int }

func (f fakeVideoActiveReporter) Active() int { return f.active }

func TestAdminGetSystemInfo_NoCalibrationSvc_VideoSessionsUnset(t *testing.T) {
	d := newTestSystemHandler()
	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if resp.VideoSessions != nil {
		t.Error("expected VideoSessions unset when no calibration service is wired")
	}
}

func TestAdminGetSystemInfo_VideoSessions_AutoOnly(t *testing.T) {
	d := newTestSystemHandler()
	d.calibrationSvc = fakeCalibrationSnapshot{capability: domaintranscoding.EncodeCapability{
		BenchSamples: []domaintranscoding.BenchRungSample{
			{Ladder: "software", Height: 1080, FPS: 60, RequiredFPS: 30, Passing: true},
		},
	}}
	d.videoLimiter = fakeVideoActiveReporter{active: 1}

	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if resp.VideoSessions == nil {
		t.Fatal("expected VideoSessions to be populated")
	}
	if resp.VideoSessions.AutoCapacity != 2 {
		t.Errorf("AutoCapacity = %d, want 2 (ratio 60/30)", resp.VideoSessions.AutoCapacity)
	}
	if resp.VideoSessions.EffectiveLimit != 2 {
		t.Errorf("EffectiveLimit = %d, want 2 (no admin override configured)", resp.VideoSessions.EffectiveLimit)
	}
	if resp.VideoSessions.Active != 1 {
		t.Errorf("Active = %d, want 1", resp.VideoSessions.Active)
	}
	if resp.VideoSessions.Evidence == nil {
		t.Fatal("expected Evidence to be populated")
	}
	if !resp.VideoSessions.Evidence.Derived {
		t.Error("expected Evidence.Derived=true with a passing bench sample")
	}
	if resp.VideoSessions.Evidence.Height != 1080 {
		t.Errorf("Evidence.Height = %d, want 1080", resp.VideoSessions.Evidence.Height)
	}
}

func TestAdminGetSystemInfo_VideoSessions_AdminOverrideReduces(t *testing.T) {
	d := newTestSystemHandler()
	d.calibrationSvc = fakeCalibrationSnapshot{capability: domaintranscoding.EncodeCapability{
		BenchSamples: []domaintranscoding.BenchRungSample{
			{Ladder: "software", Height: 1080, FPS: 90, RequiredFPS: 30, Passing: true}, // auto=3
		},
	}}
	cfg := sharedfakes.NewFakeServerConfigRepository()
	if err := cfg.Set(context.Background(), apptranscoding.MaxVideoSessionsConfigKey, "1"); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	d.serverCfgSvc = appconfig.NewService(cfg)

	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if resp.VideoSessions.AutoCapacity != 3 {
		t.Errorf("AutoCapacity = %d, want 3", resp.VideoSessions.AutoCapacity)
	}
	if resp.VideoSessions.EffectiveLimit != 1 {
		t.Errorf("EffectiveLimit = %d, want 1 (admin-reduced)", resp.VideoSessions.EffectiveLimit)
	}
}

func TestAdminGetSystemInfo_VideoSessions_NoPassingSample_FailSafe(t *testing.T) {
	d := newTestSystemHandler()
	d.calibrationSvc = fakeCalibrationSnapshot{capability: domaintranscoding.EncodeCapability{}}

	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if resp.VideoSessions.AutoCapacity != 1 {
		t.Errorf("AutoCapacity = %d, want 1 (fail-safe)", resp.VideoSessions.AutoCapacity)
	}
	if resp.VideoSessions.Evidence.Derived {
		t.Error("expected Evidence.Derived=false with no bench samples")
	}
}

func TestAdminGetServerConfig(t *testing.T) {
	d := newTestSystemHandler()
	resp, err := d.GetServerConfig(context.Background(), &eveyv1.GetServerConfigRequest{})
	if err != nil {
		t.Fatalf("GetServerConfig: %v", err)
	}
	if resp.Values == nil {
		t.Error("expected non-nil values map")
	}
}

func TestAdminUpdateServerConfig(t *testing.T) {
	d := newTestSystemHandler()
	vals := map[string]string{"key": "value"}
	resp, err := d.UpdateServerConfig(context.Background(), &eveyv1.UpdateServerConfigRequest{Values: vals})
	if err != nil {
		t.Fatalf("UpdateServerConfig: %v", err)
	}
	if resp.Values["key"] != "value" {
		t.Errorf("expected value 'value', got %q", resp.Values["key"])
	}
}

func TestAdminSetupWizard(t *testing.T) {
	d := newTestSystemHandler()
	resp, err := d.SetupWizard(context.Background(), &eveyv1.SetupWizardRequest{
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

	_, err = d.SetupWizard(context.Background(), &eveyv1.SetupWizardRequest{
		AdminUsername: "admin2",
		AdminPassword: "password456",
		AdminEmail:    "admin2@test.com",
	})
	if err == nil {
		t.Error("expected error on duplicate setup")
	}
}

func TestAdminSetupWizard_PostSetupRejected(t *testing.T) {
	d := newTestSystemHandler()
	// First setup — legitimate.
	if _, err := d.SetupWizard(context.Background(), &eveyv1.SetupWizardRequest{
		AdminUsername: "admin",
		AdminPassword: "password123",
		AdminEmail:    "admin@test.com",
	}); err != nil {
		t.Fatalf("first SetupWizard: %v", err)
	}

	// Second call with ServerLanguage/Region provided MUST reject with
	// AlreadyExists BEFORE any mutation. The source of truth is that Setup()
	// is invoked first in the handler; this test guards against regression
	// if someone reorders the calls again.
	_, err := d.SetupWizard(context.Background(), &eveyv1.SetupWizardRequest{
		AdminUsername:  "attacker",
		AdminPassword:  "irrelevant",
		AdminEmail:     "x@x",
		ServerLanguage: "attacker-locale",
		ServerRegion:   "attacker-region",
	})
	if err == nil {
		t.Fatal("expected error — setup already done")
	}
	if code := status.Code(err); code != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got %v", code)
	}
}

func TestAdminGetDashboard(t *testing.T) {
	d := newTestSystemHandler()
	resp, err := d.GetDashboard(context.Background(), &eveyv1.GetDashboardRequest{})
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

func TestAdminGetDashboard_WithLibLister(t *testing.T) {
	d := newTestSystemHandler()
	d.libLister = &fakeAdminLibLister{
		libs: []*domainlibrary.Library{
			{ID: "lib-1", Paths: []string{"/tmp"}},
		},
	}
	resp, err := d.GetDashboard(context.Background(), &eveyv1.GetDashboardRequest{})
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if len(resp.Storage) == 0 {
		t.Log("no storage info for /tmp (may be expected in CI)")
	}
}
