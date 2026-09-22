package plugin

import (
	"context"
	"testing"

	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// Stored overrides win over the boot defaults only when they parse and sit in
// range; a bad row keeps the default and must never break plugin startup.

type recordingController struct {
	memBytes, diskBytes int64
	pidsMax, nice       int
	sandbox             string
}

func (c *recordingController) SetResourceLimits(memBytes int64, pidsMax int) {
	c.memBytes, c.pidsMax = memBytes, pidsMax
}
func (c *recordingController) SetCPUNice(nice int)         { c.nice = nice }
func (c *recordingController) SetDiskQuota(maxBytes int64) { c.diskBytes = maxBytes }
func (c *recordingController) SetSandboxMode(mode string)  { c.sandbox = mode }

var bootDefaults = ResourceDefaults{MemoryMB: 256, PIDsMax: 64, DiskMaxMB: 512, CPUNice: 10, SandboxMode: "auto"}

func TestResolveResourceConfig(t *testing.T) {
	ctx := context.Background()
	if got := ResolveResourceConfig(ctx, nil, bootDefaults); got != ResourceValues(bootDefaults) {
		t.Errorf("nil repo: got %+v, want the defaults", got)
	}

	repo := sharedfakes.NewFakeServerConfigRepository()
	for k, v := range map[string]string{
		KeyMemoryMB:    "1024",
		KeyPIDsMax:     "not-a-number", // unparsable -> default
		KeyDiskMaxMB:   "-5",           // out of range -> default
		KeyCPUNice:     "19",
		KeySandboxMode: "strict",
	} {
		if err := repo.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	got := ResolveResourceConfig(ctx, repo, bootDefaults)
	want := ResourceValues{MemoryMB: 1024, PIDsMax: 64, DiskMaxMB: 512, CPUNice: 19, SandboxMode: "strict"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// nice above 19 and an unknown sandbox mode both keep the default.
	_ = repo.Set(ctx, KeyCPUNice, "20")
	_ = repo.Set(ctx, KeySandboxMode, "chroot")
	got = ResolveResourceConfig(ctx, repo, bootDefaults)
	if got.CPUNice != 10 || got.SandboxMode != "auto" {
		t.Errorf("out-of-range values must keep the defaults: %+v", got)
	}
	// An unreadable store is the same as no override.
	repo.Err = context.DeadlineExceeded
	if got := ResolveResourceConfig(ctx, repo, bootDefaults); got != ResourceValues(bootDefaults) {
		t.Errorf("failing repo: got %+v, want the defaults", got)
	}
}

func TestIsValidSandboxMode(t *testing.T) {
	for _, m := range []string{"auto", "landlock", "bubblewrap", "strict", "basic"} {
		if !IsValidSandboxMode(m) {
			t.Errorf("%q should be valid", m)
		}
	}
	for _, m := range []string{"", "chroot", "Auto", "none"} {
		if IsValidSandboxMode(m) {
			t.Errorf("%q should be rejected", m)
		}
	}
}

func TestApplyResourceConfig_PushesEffectiveValuesToTheRunner(t *testing.T) {
	ctx := context.Background()
	repo := sharedfakes.NewFakeServerConfigRepository()
	_ = repo.Set(ctx, KeyMemoryMB, "300")
	_ = repo.Set(ctx, KeyDiskMaxMB, "2")
	ctrl := &recordingController{}

	got := ApplyResourceConfig(ctx, repo, ctrl, bootDefaults)
	if got.MemoryMB != 300 || got.DiskMaxMB != 2 {
		t.Fatalf("resolved %+v", got)
	}
	if ctrl.memBytes != 300*1024*1024 || ctrl.diskBytes != 2*1024*1024 {
		t.Errorf("MiB must reach the runner as bytes: mem=%d disk=%d", ctrl.memBytes, ctrl.diskBytes)
	}
	if ctrl.pidsMax != 64 || ctrl.nice != 10 || ctrl.sandbox != "auto" {
		t.Errorf("defaults must reach the runner too: %+v", ctrl)
	}
	// A nil controller only resolves.
	if got := ApplyResourceConfig(ctx, repo, nil, bootDefaults); got.MemoryMB != 300 {
		t.Errorf("nil controller: got %+v", got)
	}
}
