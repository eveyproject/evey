package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	transcodingfakes "github.com/evey-project/evey/internal/domain/transcoding/fakes"
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
