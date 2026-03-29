package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	transcodingfakes "github.com/gravity-zero/evey/internal/domain/transcoding/fakes"
)

func TestAdminGetSetupStatus_NoAdmins(t *testing.T) {
	d := newTestSystemDelegate()
	resp, err := d.GetSetupStatus(context.Background(), &eveyv1.GetSetupStatusRequest{})
	if err != nil {
		t.Fatalf("GetSetupStatus: %v", err)
	}
	if resp.SetupComplete {
		t.Error("expected SetupComplete=false with no admins")
	}
}

func TestAdminGetSystemInfo(t *testing.T) {
	d := newTestSystemDelegate()
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
	d := newTestSystemDelegate()
	hwDetector := transcodingfakes.NewFakeHWDetector()
	events := sharedfakes.NewFakeEventBus()
	d.transcodingSvc = apptranscoding.NewService(nil, hwDetector, nil, nil, events, 1)

	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	_ = resp
}

func TestAdminGetServerConfig(t *testing.T) {
	d := newTestSystemDelegate()
	resp, err := d.GetServerConfig(context.Background(), &eveyv1.GetServerConfigRequest{})
	if err != nil {
		t.Fatalf("GetServerConfig: %v", err)
	}
	if resp.Values == nil {
		t.Error("expected non-nil values map")
	}
}

func TestAdminUpdateServerConfig(t *testing.T) {
	d := newTestSystemDelegate()
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
	d := newTestSystemDelegate()
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
	d := newTestSystemDelegate()
	tmpDir := t.TempDir()
	hls := apptranscoding.NewHLSManager(context.Background(), tmpDir, 5, &transcodingfakes.HLSProcess{})
	defer hls.CleanupAll()
	d.hlsManager = hls

	resp, err := d.GetDashboard(context.Background(), &eveyv1.GetDashboardRequest{})
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if resp.ActiveTranscodes != 0 {
		t.Errorf("expected 0 active transcodes, got %d", resp.ActiveTranscodes)
	}
}

func TestAdminGetDashboard_WithLibLister(t *testing.T) {
	d := newTestSystemDelegate()
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
