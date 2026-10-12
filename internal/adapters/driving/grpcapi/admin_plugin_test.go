package grpcapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// newUpdateTestHandler builds a handler + manifest parser that are wired
// for PreviewPluginUpdate / ApplyPluginUpdate tests. The caller controls
// the parser result before each call.
func newUpdateTestHandler(t *testing.T) (*adminPluginHandler, *pluginfakes.FakePluginRepository, *pluginfakes.FakeManifestParser) {
	t.Helper()
	repo := pluginfakes.NewFakePluginRepository()
	runner := pluginfakes.NewFakePluginRunner()
	parser := pluginfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := pluginfakes.NewFakePluginInstaller()
	analyzer := pluginfakes.NewFakePluginAnalyzer()
	fs := pluginfakes.NewFakePluginFileSystem()
	runtimeRepo := pluginfakes.NewFakePluginRuntimeRepository()
	analyzer.Report = &plugindomain.AnalysisReport{Safe: true}

	svc := appplugin.NewInstallerService(repo, runner, parser, bus,
		appplugin.InstallerWithInstaller(installer),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir(t.TempDir()),
		appplugin.InstallerWithFileSystem(fs),
	)
	h := &adminPluginHandler{
		pluginInstaller: svc,
		pluginRuntime:   appplugin.NewRuntimeService(repo, runtimeRepo, runner, parser, bus),
		pluginQuery:     appplugin.NewQueryService(repo, runtimeRepo),
		enabled:         true,
	}
	return h, repo, parser
}

// tmpZipGRPC creates a tiny temp zip file for handler tests.
func tmpZipGRPC(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "plugin-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestPreviewPluginUpdate_MissingPluginID(t *testing.T) {
	h, _, _ := newUpdateTestHandler(t)
	_, err := h.PreviewPluginUpdate(context.Background(), &eveyv1.PreviewPluginUpdateRequest{
		PluginId: "",
		Source:   "/some/path.zip",
	})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
}

func TestPreviewPluginUpdate_MissingSource(t *testing.T) {
	h, _, _ := newUpdateTestHandler(t)
	_, err := h.PreviewPluginUpdate(context.Background(), &eveyv1.PreviewPluginUpdateRequest{
		PluginId: "plg-1",
		Source:   "",
	})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
}

func TestPreviewPluginUpdate_NotFound(t *testing.T) {
	h, _, parser := newUpdateTestHandler(t)
	parser.Result = &plugindomain.PluginManifest{
		Name: "x", Kind: plugindomain.KindPlugin, Version: "1.0.0",
		Language: "go", Entrypoint: "./p",
	}
	_, err := h.PreviewPluginUpdate(context.Background(), &eveyv1.PreviewPluginUpdateRequest{
		PluginId: "plg-ghost",
		Source:   tmpZipGRPC(t, "zip"),
	})
	if code := status.Code(err); code != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", code)
	}
}

func TestPreviewPluginUpdate_NewPermission(t *testing.T) {
	h, repo, parser := newUpdateTestHandler(t)
	ctx := context.Background()

	// Seed v1 plugin.
	if err := repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-prev", Name: "preview-me", Version: "1.0.0",
		Kind: plugindomain.KindPlugin, Language: "go", Entrypoint: "./p",
		Permissions: []string{"api.library"},
	}); err != nil {
		t.Fatal(err)
	}

	// Candidate adds "network".
	parser.Result = &plugindomain.PluginManifest{
		Name: "preview-me", Kind: plugindomain.KindPlugin, Version: "2.0.0",
		Language: "go", Entrypoint: "./p",
		Permissions: []string{"api.library", "network"},
	}

	resp, err := h.PreviewPluginUpdate(ctx, &eveyv1.PreviewPluginUpdateRequest{
		PluginId: "plg-prev",
		Source:   tmpZipGRPC(t, "v2"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q", resp.CurrentVersion)
	}
	if resp.CandidateVersion != "2.0.0" {
		t.Errorf("CandidateVersion = %q", resp.CandidateVersion)
	}
	if len(resp.PermissionsAdded) != 1 || resp.PermissionsAdded[0] != "network" {
		t.Errorf("PermissionsAdded = %v", resp.PermissionsAdded)
	}
	if !resp.RequiresAck {
		t.Error("RequiresAck should be true when permissions added")
	}
	if resp.SourceHash == "" {
		t.Error("SourceHash should not be empty")
	}
}

func TestApplyPluginUpdate_MissingAcknowledgedHash(t *testing.T) {
	h, _, _ := newUpdateTestHandler(t)
	_, err := h.ApplyPluginUpdate(context.Background(), &eveyv1.ApplyPluginUpdateRequest{
		PluginId: "plg-1",
		Source:   "/some/path.zip",
		// acknowledged_hash intentionally absent
	})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
}

func TestApplyPluginUpdate_HashMismatch(t *testing.T) {
	h, repo, parser := newUpdateTestHandler(t)
	ctx := context.Background()

	if err := repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-apply", Name: "apply-me", Version: "1.0.0",
		Kind: plugindomain.KindPlugin, Language: "go", Entrypoint: "./p",
	}); err != nil {
		t.Fatal(err)
	}

	parser.Result = &plugindomain.PluginManifest{
		Name: "apply-me", Kind: plugindomain.KindPlugin, Version: "2.0.0",
		Language: "go", Entrypoint: "./p",
	}

	_, err := h.ApplyPluginUpdate(ctx, &eveyv1.ApplyPluginUpdateRequest{
		PluginId:         "plg-apply",
		Source:           tmpZipGRPC(t, "v2"),
		AcknowledgedHash: strings.Repeat("0", 64),
	})
	if code := status.Code(err); code != codes.Aborted {
		t.Fatalf("code = %v, want Aborted (hash mismatch)", code)
	}
}

func TestApplyPluginUpdate_NotFound(t *testing.T) {
	h, _, parser := newUpdateTestHandler(t)
	parser.Result = &plugindomain.PluginManifest{
		Name: "x", Kind: plugindomain.KindPlugin, Version: "1.0.0",
		Language: "go", Entrypoint: "./p",
	}
	_, err := h.ApplyPluginUpdate(context.Background(), &eveyv1.ApplyPluginUpdateRequest{
		PluginId:         "plg-ghost",
		Source:           tmpZipGRPC(t, "v"),
		AcknowledgedHash: strings.Repeat("a", 64),
	})
	if code := status.Code(err); code != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", code)
	}
}

// GetPluginLogs joined the caller-supplied plugin id straight into a path, so
// an admin could walk out of the plugin log directory and read any *.log on the
// host. Plugin ids are server-generated ("plg-<hex>") and never contain a
// separator, so anything that does is a traversal attempt. The plugin ASSET
// handler already rejects these - this was the same check missing on its twin.
func TestGetPluginLogs_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "plugins"), 0o755))
	// A file the traversal would reach if the id were not validated.
	outside := filepath.Join(dir, "secret.log")
	require.NoError(t, os.WriteFile(outside,
		[]byte(`{"time":"2026-07-20T00:00:00Z","level":"INFO","msg":"leaked"}`+"\n"), 0o644))

	h := &adminPluginHandler{logDir: dir}

	for _, id := range []string{"../secret", "../../etc/passwd", "sub/dir", `..\secret`} {
		resp, err := h.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: id})
		if err == nil && resp != nil && len(resp.Entries) > 0 {
			t.Errorf("plugin id %q escaped the log directory and returned %d entries", id, len(resp.Entries))
		}
		// Refused outright, not merely answered empty: the glob below the check
		// happens to find nothing today, which let the check go untested.
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("plugin id %q must be refused as invalid, got %v", id, err)
		}
	}
}

func TestGetPluginLogs_StillReadsARealPluginLog(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "plugins"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugins", "plg-abc123.log"),
		[]byte(`{"time":"2026-07-20T00:00:00Z","level":"INFO","msg":"hello"}`+"\n"), 0o644))

	h := &adminPluginHandler{logDir: dir}
	resp, err := h.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "plg-abc123"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.Entries, "a legitimate plugin id must still return its log")
}
