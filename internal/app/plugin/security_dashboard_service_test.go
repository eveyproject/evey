package plugin_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

func TestGetSecurityDashboard_Empty(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "basic" })

	snaps, err := svc.GetSecurityDashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snaps))
	}
}

func TestGetSecurityDashboard_OnePlugin(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	now := time.Now().UTC().Truncate(time.Second)

	p := &plugin.Plugin{
		ID:          "plugin-1",
		Name:        "Test Plugin",
		Version:     "1.0.0",
		Permissions: []string{"network"},
		SourceHash:  "abc123",
		InstalledAt: now,
		Status:      plugin.Stopped,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "basic" })

	snaps, err := svc.GetSecurityDashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	s := snaps[0]
	if s.ID != "plugin-1" {
		t.Errorf("unexpected ID: %q", s.ID)
	}
	if s.SandboxMode != "basic" {
		t.Errorf("expected sandboxMode basic, got %q", s.SandboxMode)
	}
	if s.SourceSHA256 != "abc123" {
		t.Errorf("expected sourceSHA256 abc123, got %q", s.SourceSHA256)
	}
	if s.LastError != nil {
		t.Error("expected no last error initially")
	}
	if s.Permissions == nil {
		t.Error("permissions should not be nil")
	}
}

func TestGetSecurityDashboard_SubprocessPermission_SandboxNone(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	p := &plugin.Plugin{
		ID:          "plugin-sub",
		Name:        "Subprocess Plugin",
		Version:     "0.1.0",
		Permissions: []string{"subprocess", "network"},
		Status:      plugin.Stopped,
		InstalledAt: time.Now(),
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "bubblewrap" })

	snaps, err := svc.GetSecurityDashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].SandboxMode != "none" {
		t.Errorf("expected sandboxMode none for subprocess plugin, got %q", snaps[0].SandboxMode)
	}
}

func TestGetSecurityDashboard_LastErrorViaEventBus(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	p := &plugin.Plugin{
		ID:          "plugin-err",
		Name:        "Error Plugin",
		Version:     "1.0.0",
		Permissions: []string{},
		Status:      plugin.Error,
		InstalledAt: time.Now(),
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "basic" })

	// Simulate a plugin.error event being published.
	errEvent := plugin.PluginError{
		PluginID:     "plugin-err",
		ErrorMessage: "process crashed: signal: killed",
		OccurredAt:   time.Now(),
	}
	if err := bus.Publish(context.Background(), errEvent); err != nil {
		t.Fatal(err)
	}

	snaps, err := svc.GetSecurityDashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].LastError == nil {
		t.Fatal("expected last error to be set after plugin.error event")
	}
	if snaps[0].LastError.Message != "process crashed: signal: killed" {
		t.Errorf("unexpected error message: %q", snaps[0].LastError.Message)
	}
}

func TestGetSecurityDashboard_SizeBytes(t *testing.T) {
	dir := t.TempDir()
	// Write a file with known content.
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0600); err != nil {
		t.Fatal(err)
	}

	repo := fakes.NewFakePluginRepository()
	p := &plugin.Plugin{
		ID:          "plugin-size",
		Name:        "Size Plugin",
		Version:     "1.0.0",
		Permissions: []string{},
		Status:      plugin.Stopped,
		InstalledAt: time.Now(),
		InstallDir:  dir,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "basic" })

	snaps, err := svc.GetSecurityDashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].SizeBytes <= 0 {
		t.Errorf("expected sizeBytes > 0, got %d", snaps[0].SizeBytes)
	}
}

func TestGetSecurityDashboard_Findings(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	p := &plugin.Plugin{
		ID:          "plugin-findings",
		Name:        "Findings Plugin",
		Version:     "1.0.0",
		Permissions: []string{},
		Status:      plugin.Stopped,
		InstalledAt: time.Now(),
		AnalysisReport: &plugin.AnalysisReport{
			Safe:       false,
			Issues:     []string{"[CRITICAL] eval.py:5 - dynamic code execution (eval/exec) (eval())", "[WARNING] base.py:12 - base64 decoding (atob(x))"},
			Categories: []string{"obfuscation", "obfuscation"},
		},
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "basic" })

	snaps, err := svc.GetSecurityDashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if len(snaps[0].Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(snaps[0].Findings))
	}
	if snaps[0].Findings[0].Severity != "CRITICAL" {
		t.Errorf("expected first finding severity CRITICAL, got %q", snaps[0].Findings[0].Severity)
	}
}

func TestGetSecurityDashboard_RepoError(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	repo.Err = context.DeadlineExceeded

	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "basic" })

	_, err := svc.GetSecurityDashboard(context.Background())
	if err == nil {
		t.Fatal("expected error when repo fails")
	}
}

func TestSandboxStatus_FallbackFromModeFn(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "basic" })

	st := svc.SandboxStatus()
	if st.Effective != "basic" || st.Configured != "basic" {
		t.Errorf("fallback status should mirror sandboxModeFn, got %+v", st)
	}
	if st.LandlockKernel || st.LauncherPresent || st.BubblewrapUsable {
		t.Errorf("fallback status should not invent probe results, got %+v", st)
	}
}

func TestSandboxStatus_WithSandboxStatus(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	bus := sharedfakes.NewFakeEventBus()
	want := appplugin.SandboxStatus{
		Configured:      "auto",
		Effective:       "landlock",
		LandlockKernel:  true,
		LauncherPresent: true,
	}
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "landlock" }).
		WithSandboxStatus(func() appplugin.SandboxStatus { return want })

	if got := svc.SandboxStatus(); got != want {
		t.Errorf("SandboxStatus: want %+v, got %+v", want, got)
	}
}

// A plugin.started event stamps the snapshot's LastStartedAt for THAT plugin
// only; a foreign event on the same topic is ignored.
func TestGetSecurityDashboard_LastStartedViaEventBus(t *testing.T) {
	repo := fakes.NewFakePluginRepository()
	for _, id := range []string{"plugin-a", "plugin-b"} {
		if err := repo.Create(context.Background(), &plugin.Plugin{ID: id, Name: id, Version: "1.0.0", InstalledAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewSecurityDashboardService(repo, bus, func() string { return "basic" })

	started := time.Date(2026, 9, 16, 1, 30, 0, 0, time.UTC)
	if err := bus.Publish(context.Background(), plugin.PluginStarted{PluginID: "plugin-a", OccurredAt: started}); err != nil {
		t.Fatal(err)
	}
	// Same topic, wrong payload type: must be ignored, not panic.
	if err := bus.Publish(context.Background(), plugin.PluginError{PluginID: "plugin-b", ErrorMessage: "x", OccurredAt: started}); err != nil {
		t.Fatal(err)
	}

	snaps, err := svc.GetSecurityDashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]plugin.PluginSecuritySnapshot{}
	for _, s := range snaps {
		byID[s.ID] = s
	}
	if got := byID["plugin-a"].LastStartedAt; got == nil || !got.Equal(started) {
		t.Errorf("plugin-a LastStartedAt = %v, want %v", got, started)
	}
	if byID["plugin-b"].LastStartedAt != nil {
		t.Errorf("plugin-b never started, got %v", byID["plugin-b"].LastStartedAt)
	}
}
