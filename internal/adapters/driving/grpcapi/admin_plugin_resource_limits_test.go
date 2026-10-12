package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakePluginResourceController implements pluginResourceController for tests.
// It returns fixed values from ResourceConfig and records calls to the setter
// methods so that ApplyResourceConfig can push updates into it.
type fakePluginResourceController struct {
	mem     int
	pids    int
	disk    int
	nice    int
	sandbox string
}

func (f *fakePluginResourceController) ResourceConfig() (memMB, pidsMax, diskMB, nice int, sandboxMode string) {
	return f.mem, f.pids, f.disk, f.nice, f.sandbox
}
func (f *fakePluginResourceController) SetResourceLimits(memBytes int64, pidsMax int) {
	f.mem = int(memBytes / 1024 / 1024)
	f.pids = pidsMax
}
func (f *fakePluginResourceController) SetDiskQuota(maxBytes int64) {
	f.disk = int(maxBytes / 1024 / 1024)
}
func (f *fakePluginResourceController) SetCPUNice(nice int)        { f.nice = nice }
func (f *fakePluginResourceController) SetSandboxMode(mode string) { f.sandbox = mode }

// newResourceLimitsHandler returns a handler wired with a live fakePluginResourceController
// and a fresh FakeServerConfigRepository.
func newResourceLimitsHandler(res *fakePluginResourceController) *adminPluginHandler {
	return &adminPluginHandler{
		pluginRes:   res,
		serverCfg:   sharedfakes.NewFakeServerConfigRepository(),
		resDefaults: appplugin.ResourceDefaults{},
		enabled:     true,
	}
}

// TestEffectiveResourceLimits_WithRunner verifies that when pluginRes is set,
// effectiveResourceLimits reads the runner's live values directly.
func TestEffectiveResourceLimits_WithRunner(t *testing.T) {
	res := &fakePluginResourceController{
		mem: 512, pids: 100, disk: 256, nice: 10, sandbox: "landlock",
	}
	h := newResourceLimitsHandler(res)

	got := h.effectiveResourceLimits(context.Background())

	if got.MemoryMb != 512 {
		t.Errorf("MemoryMb = %d, want 512", got.MemoryMb)
	}
	if got.PidsMax != 100 {
		t.Errorf("PidsMax = %d, want 100", got.PidsMax)
	}
	if got.DiskMaxMb != 256 {
		t.Errorf("DiskMaxMb = %d, want 256", got.DiskMaxMb)
	}
	if got.CpuNice != 10 {
		t.Errorf("CpuNice = %d, want 10", got.CpuNice)
	}
	if got.SandboxMode != "landlock" {
		t.Errorf("SandboxMode = %q, want %q", got.SandboxMode, "landlock")
	}
}

// TestEffectiveResourceLimits_NoRunner verifies that when pluginRes is nil,
// effectiveResourceLimits falls back to ResolveResourceConfig over the store.
func TestEffectiveResourceLimits_NoRunner(t *testing.T) {
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	h := &adminPluginHandler{
		pluginRes: nil,
		serverCfg: cfgRepo,
		resDefaults: appplugin.ResourceDefaults{
			MemoryMB: 128, PIDsMax: 50, DiskMaxMB: 64, CPUNice: 5, SandboxMode: "auto",
		},
		enabled: true,
	}

	got := h.effectiveResourceLimits(context.Background())

	// No stored overrides - must equal the defaults.
	if got.MemoryMb != 128 {
		t.Errorf("MemoryMb = %d, want 128", got.MemoryMb)
	}
	if got.PidsMax != 50 {
		t.Errorf("PidsMax = %d, want 50", got.PidsMax)
	}
	if got.DiskMaxMb != 64 {
		t.Errorf("DiskMaxMb = %d, want 64", got.DiskMaxMb)
	}
	if got.CpuNice != 5 {
		t.Errorf("CpuNice = %d, want 5", got.CpuNice)
	}
	if got.SandboxMode != "auto" {
		t.Errorf("SandboxMode = %q, want %q", got.SandboxMode, "auto")
	}
}

// TestEffectiveResourceLimits_NoRunner_StoredOverride verifies that a stored
// override in the repo is picked up when the runner is absent.
func TestEffectiveResourceLimits_NoRunner_StoredOverride(t *testing.T) {
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	ctx := context.Background()
	if err := cfgRepo.Set(ctx, appplugin.KeyMemoryMB, "1024"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := &adminPluginHandler{
		pluginRes:   nil,
		serverCfg:   cfgRepo,
		resDefaults: appplugin.ResourceDefaults{MemoryMB: 128},
		enabled:     true,
	}

	got := h.effectiveResourceLimits(ctx)

	if got.MemoryMb != 1024 {
		t.Errorf("MemoryMb = %d, want 1024 (stored override)", got.MemoryMb)
	}
}

// TestGetPluginResourceLimits_ReturnsEffective verifies the RPC returns the
// runner's live values when authenticated.
func TestGetPluginResourceLimits_ReturnsEffective(t *testing.T) {
	res := &fakePluginResourceController{
		mem: 256, pids: 32, disk: 512, nice: 0, sandbox: "basic",
	}
	h := newResourceLimitsHandler(res)
	ctx := ctxWithUser("usr-admin")

	resp, err := h.GetPluginResourceLimits(ctx, &eveyv1.GetPluginResourceLimitsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MemoryMb != 256 {
		t.Errorf("MemoryMb = %d, want 256", resp.MemoryMb)
	}
	if resp.SandboxMode != "basic" {
		t.Errorf("SandboxMode = %q, want %q", resp.SandboxMode, "basic")
	}
}

// TestGetPluginResourceLimits_Unauthenticated verifies that a missing claims
// context yields Unauthenticated.
func TestGetPluginResourceLimits_Unauthenticated(t *testing.T) {
	h := newResourceLimitsHandler(&fakePluginResourceController{})
	_, err := h.GetPluginResourceLimits(context.Background(), &eveyv1.GetPluginResourceLimitsRequest{})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", code)
	}
}

// TestUpdatePluginResourceLimits_HappyPath persists new caps and returns them.
func TestUpdatePluginResourceLimits_HappyPath(t *testing.T) {
	res := &fakePluginResourceController{
		mem: 128, pids: 10, disk: 64, nice: 0, sandbox: "auto",
	}
	h := newResourceLimitsHandler(res)
	ctx := ctxWithUser("usr-admin")

	req := &eveyv1.PluginResourceLimits{
		MemoryMb:    1024,
		PidsMax:     200,
		DiskMaxMb:   512,
		CpuNice:     5,
		SandboxMode: "landlock",
	}
	resp, err := h.UpdatePluginResourceLimits(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ApplyResourceConfig pushes the new values into the fake runner, so
	// effectiveResourceLimits (via ResourceConfig) reflects them.
	if resp.MemoryMb != 1024 {
		t.Errorf("MemoryMb = %d, want 1024", resp.MemoryMb)
	}
	if resp.PidsMax != 200 {
		t.Errorf("PidsMax = %d, want 200", resp.PidsMax)
	}
	if resp.DiskMaxMb != 512 {
		t.Errorf("DiskMaxMb = %d, want 512", resp.DiskMaxMb)
	}
	if resp.CpuNice != 5 {
		t.Errorf("CpuNice = %d, want 5", resp.CpuNice)
	}
	if resp.SandboxMode != "landlock" {
		t.Errorf("SandboxMode = %q, want %q", resp.SandboxMode, "landlock")
	}
}

// TestUpdatePluginResourceLimits_Unauthenticated verifies missing claims yields Unauthenticated.
func TestUpdatePluginResourceLimits_Unauthenticated(t *testing.T) {
	h := newResourceLimitsHandler(&fakePluginResourceController{})
	_, err := h.UpdatePluginResourceLimits(context.Background(), &eveyv1.PluginResourceLimits{})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", code)
	}
}

// TestUpdatePluginResourceLimits_NilRequest verifies a nil request yields InvalidArgument.
func TestUpdatePluginResourceLimits_NilRequest(t *testing.T) {
	h := newResourceLimitsHandler(&fakePluginResourceController{})
	ctx := ctxWithUser("usr-admin")
	_, err := h.UpdatePluginResourceLimits(ctx, nil)
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
}

// TestUpdatePluginResourceLimits_NegativeMemory verifies out-of-range values are rejected.
func TestUpdatePluginResourceLimits_NegativeMemory(t *testing.T) {
	h := newResourceLimitsHandler(&fakePluginResourceController{})
	ctx := ctxWithUser("usr-admin")
	_, err := h.UpdatePluginResourceLimits(ctx, &eveyv1.PluginResourceLimits{MemoryMb: -1})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
}

// TestUpdatePluginResourceLimits_BadNice verifies cpu_nice > 19 is rejected.
func TestUpdatePluginResourceLimits_BadNice(t *testing.T) {
	h := newResourceLimitsHandler(&fakePluginResourceController{})
	ctx := ctxWithUser("usr-admin")
	_, err := h.UpdatePluginResourceLimits(ctx, &eveyv1.PluginResourceLimits{CpuNice: 20})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
}

// TestUpdatePluginResourceLimits_BadSandboxMode verifies an unknown sandbox mode is rejected.
func TestUpdatePluginResourceLimits_BadSandboxMode(t *testing.T) {
	h := newResourceLimitsHandler(&fakePluginResourceController{})
	ctx := ctxWithUser("usr-admin")
	_, err := h.UpdatePluginResourceLimits(ctx, &eveyv1.PluginResourceLimits{SandboxMode: "unknown"})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
}

// TestUpdatePluginResourceLimits_NoServerCfg verifies FailedPrecondition when serverCfg is nil.
func TestUpdatePluginResourceLimits_NoServerCfg(t *testing.T) {
	h := &adminPluginHandler{
		pluginRes: &fakePluginResourceController{},
		serverCfg: nil,
		enabled:   true,
	}
	ctx := ctxWithUser("usr-admin")
	_, err := h.UpdatePluginResourceLimits(ctx, &eveyv1.PluginResourceLimits{SandboxMode: "auto"})
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", code)
	}
}
