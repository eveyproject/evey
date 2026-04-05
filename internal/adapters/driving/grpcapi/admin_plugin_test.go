package grpcapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
)

func TestAdminListPluginRuntimes(t *testing.T) {
	d := newTestPluginHandler()
	resp, err := d.ListPluginRuntimes(context.Background(), &eveyv1.ListPluginRuntimesRequest{})
	if err != nil {
		t.Fatalf("ListPluginRuntimes: %v", err)
	}
	if len(resp.Runtimes) != 0 {
		t.Errorf("expected 0 runtimes, got %d", len(resp.Runtimes))
	}
}

func TestAdminAddPluginRuntime(t *testing.T) {
	d := newTestPluginHandler()
	resp, err := d.AddPluginRuntime(context.Background(), &eveyv1.AddPluginRuntimeRequest{
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
	d := newTestPluginHandler()
	d.AddPluginRuntime(context.Background(), &eveyv1.AddPluginRuntimeRequest{
		Name: "python3", Language: "python", Version: "3.11",
	})
	_, err := d.RemovePluginRuntime(context.Background(), &eveyv1.RemovePluginRuntimeRequest{Id: "python3"})
	if err != nil {
		t.Fatalf("RemovePluginRuntime: %v", err)
	}
}

func TestAdminListPlugins(t *testing.T) {
	d := newTestPluginHandler()
	resp, err := d.ListPlugins(context.Background(), &eveyv1.ListPluginsRequest{})
	if err != nil {
		t.Fatalf("ListPlugins: %v", err)
	}
	if len(resp.Plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(resp.Plugins))
	}
}

func TestAdminEnablePlugin(t *testing.T) {
	d := newTestPluginHandler()
	_, err := d.EnablePlugin(context.Background(), &eveyv1.EnablePluginRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestAdminDisablePlugin(t *testing.T) {
	d := newTestPluginHandler()
	_, err := d.DisablePlugin(context.Background(), &eveyv1.DisablePluginRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestAdminGetPluginConfig(t *testing.T) {
	d := newTestPluginHandler()
	_, err := d.GetPluginConfig(context.Background(), &eveyv1.GetPluginConfigRequest{PluginId: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent plugin config")
	}
}

func TestAdminUpdatePluginConfig(t *testing.T) {
	d := newTestPluginHandler()
	_, err := d.UpdatePluginConfig(context.Background(), &eveyv1.UpdatePluginConfigRequest{
		PluginId: "nonexistent",
		Values:   map[string]string{"key": "value"},
	})
	if err == nil {
		t.Error("expected error for nonexistent plugin config")
	}
}

func TestAdminGetPluginLogs_NoLogDir(t *testing.T) {
	d := newTestPluginHandler()
	resp, err := d.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "p1"})
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

	d := newTestPluginHandler()
	d.logDir = tmpDir

	resp, err := d.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "p1"})
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

	d := newTestPluginHandler()
	d.logDir = tmpDir

	resp, err := d.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "p2", Level: "ERROR"})
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

	d := newTestPluginHandler()
	d.logDir = tmpDir

	resp, err := d.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "p3", Offset: 2, Limit: 2})
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
