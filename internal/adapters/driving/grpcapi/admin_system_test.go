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
type fakeVideoActiveReporter struct {
	active int
	used   float64
	budget float64
	rungs  []apptranscoding.RungFit
	// poolUsed/poolBudget/poolRungs/classes let tests simulate the per-pool
	// breakdown (UsedFor/BudgetFor/RungsFor/Classes) independently of the
	// aggregate Used()/Budget()/Rungs() above.
	poolUsed   map[apptranscoding.ResourcePool]float64
	poolBudget map[apptranscoding.ResourcePool]float64
	poolRungs  map[apptranscoding.LadderClass][]apptranscoding.RungFit
	classes    []apptranscoding.LadderClass
}

func (f fakeCalibrationSnapshot) IsBenchmarking() bool { return false }

func (f fakeVideoActiveReporter) Active() int                     { return f.active }
func (f fakeVideoActiveReporter) Used() float64                   { return f.used }
func (f fakeVideoActiveReporter) Budget() float64                 { return f.budget }
func (f fakeVideoActiveReporter) Rungs() []apptranscoding.RungFit { return f.rungs }

func (f fakeVideoActiveReporter) UsedFor(pool apptranscoding.ResourcePool) float64 {
	return f.poolUsed[pool]
}

func (f fakeVideoActiveReporter) BudgetFor(pool apptranscoding.ResourcePool) float64 {
	return f.poolBudget[pool]
}

func (f fakeVideoActiveReporter) RungsFor(class apptranscoding.LadderClass) []apptranscoding.RungFit {
	return f.poolRungs[class]
}

func (f fakeVideoActiveReporter) Classes() []apptranscoding.LadderClass {
	return f.classes
}

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

func TestAdminGetSystemInfo_VideoSessions_BudgetAndRungFits(t *testing.T) {
	d := newTestSystemHandler()
	d.calibrationSvc = fakeCalibrationSnapshot{capability: domaintranscoding.EncodeCapability{
		BenchSamples: []domaintranscoding.BenchRungSample{
			{Ladder: "software", Height: 1080, FPS: 60, RequiredFPS: 30, Passing: true}, // auto=2
		},
	}}
	d.videoLimiter = fakeVideoActiveReporter{
		active: 2,
		used:   0.961,
		budget: 1.0,
		rungs: []apptranscoding.RungFit{
			{Height: 1080, Cost: 0.431, MaxStreams: 2},
			{Height: 1440, Cost: 0.530, MaxStreams: 1},
		},
	}

	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if resp.VideoSessions == nil {
		t.Fatal("expected VideoSessions to be populated")
	}
	if resp.VideoSessions.BudgetUsed != 0.961 {
		t.Errorf("BudgetUsed = %v, want 0.961", resp.VideoSessions.BudgetUsed)
	}
	if resp.VideoSessions.BudgetTotal != 1.0 {
		t.Errorf("BudgetTotal = %v, want 1.0", resp.VideoSessions.BudgetTotal)
	}
	if len(resp.VideoSessions.RungFits) != 2 {
		t.Fatalf("RungFits len = %d, want 2", len(resp.VideoSessions.RungFits))
	}
	if resp.VideoSessions.RungFits[0].Height != 1080 || resp.VideoSessions.RungFits[0].MaxStreams != 2 {
		t.Errorf("RungFits[0] = %+v, want height=1080 maxStreams=2", resp.VideoSessions.RungFits[0])
	}
	if resp.VideoSessions.RungFits[1].Height != 1440 || resp.VideoSessions.RungFits[1].MaxStreams != 1 {
		t.Errorf("RungFits[1] = %+v, want height=1440 maxStreams=1", resp.VideoSessions.RungFits[1])
	}
}

// A box with no hw bench table (Classes() reports only software) must never
// surface a hollow "gpu" pool row - the panel would otherwise show a GPU
// budget on a host that has none.
func TestAdminGetSystemInfo_VideoSessions_Pools_NoHWTable_NoGPUPool(t *testing.T) {
	d := newTestSystemHandler()
	d.calibrationSvc = fakeCalibrationSnapshot{capability: domaintranscoding.EncodeCapability{
		BenchSamples: []domaintranscoding.BenchRungSample{
			{Ladder: "software", Height: 1080, FPS: 60, RequiredFPS: 30, Passing: true},
		},
	}}
	d.videoLimiter = fakeVideoActiveReporter{
		classes:    []apptranscoding.LadderClass{apptranscoding.LadderClassSoftware},
		poolUsed:   map[apptranscoding.ResourcePool]float64{apptranscoding.PoolCPU: 0.431},
		poolBudget: map[apptranscoding.ResourcePool]float64{apptranscoding.PoolCPU: 1.0},
		poolRungs: map[apptranscoding.LadderClass][]apptranscoding.RungFit{
			apptranscoding.LadderClassSoftware: {{Height: 1080, Cost: 0.431, MaxStreams: 2}},
		},
	}

	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if len(resp.VideoSessions.Pools) != 1 {
		t.Fatalf("Pools len = %d, want 1 (cpu only)", len(resp.VideoSessions.Pools))
	}
	cpu := resp.VideoSessions.Pools[0]
	if cpu.Pool != "cpu" {
		t.Errorf("Pools[0].Pool = %q, want cpu", cpu.Pool)
	}
	if cpu.BudgetUsed != 0.431 || cpu.BudgetTotal != 1.0 {
		t.Errorf("cpu pool budgets = %v/%v, want 0.431/1.0", cpu.BudgetUsed, cpu.BudgetTotal)
	}
	if len(cpu.Classes) != 1 || cpu.Classes[0].LadderClass != "software" {
		t.Fatalf("cpu pool classes = %+v, want one software class", cpu.Classes)
	}
	if len(cpu.Classes[0].RungFits) != 1 || cpu.Classes[0].RungFits[0].Height != 1080 {
		t.Errorf("cpu software rung_fits = %+v, want [height=1080]", cpu.Classes[0].RungFits)
	}
}

// A box WITH a hw bench table gets both pools, each with its own independent
// budget - the whole point of pools existing instead of the misleading
// aggregate budget_used/budget_total sum.
func TestAdminGetSystemInfo_VideoSessions_Pools_WithHWTable_BothPools(t *testing.T) {
	d := newTestSystemHandler()
	d.calibrationSvc = fakeCalibrationSnapshot{capability: domaintranscoding.EncodeCapability{
		BenchSamples: []domaintranscoding.BenchRungSample{
			{Ladder: "software", Height: 1080, FPS: 60, RequiredFPS: 30, Passing: true},
		},
	}}
	d.videoLimiter = fakeVideoActiveReporter{
		classes: []apptranscoding.LadderClass{apptranscoding.LadderClassHW, apptranscoding.LadderClassSoftware},
		poolUsed: map[apptranscoding.ResourcePool]float64{
			apptranscoding.PoolCPU: 0.431,
			apptranscoding.PoolGPU: 0.980,
		},
		poolBudget: map[apptranscoding.ResourcePool]float64{
			apptranscoding.PoolCPU: 1.0,
			apptranscoding.PoolGPU: 1.0,
		},
		poolRungs: map[apptranscoding.LadderClass][]apptranscoding.RungFit{
			apptranscoding.LadderClassSoftware: {{Height: 1080, Cost: 0.431, MaxStreams: 2}},
			apptranscoding.LadderClassHW:       {{Height: 1080, Cost: 0.245, MaxStreams: 4}},
		},
	}

	resp, err := d.GetSystemInfo(context.Background(), &eveyv1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if len(resp.VideoSessions.Pools) != 2 {
		t.Fatalf("Pools len = %d, want 2 (cpu + gpu)", len(resp.VideoSessions.Pools))
	}

	var cpu, gpu *eveyv1.VideoPool
	for _, p := range resp.VideoSessions.Pools {
		switch p.Pool {
		case "cpu":
			cpu = p
		case "gpu":
			gpu = p
		}
	}
	if cpu == nil || gpu == nil {
		t.Fatalf("expected both cpu and gpu pools, got %+v", resp.VideoSessions.Pools)
	}
	if cpu.BudgetUsed != 0.431 || cpu.BudgetTotal != 1.0 {
		t.Errorf("cpu pool budgets = %v/%v, want 0.431/1.0", cpu.BudgetUsed, cpu.BudgetTotal)
	}
	if gpu.BudgetUsed != 0.980 || gpu.BudgetTotal != 1.0 {
		t.Errorf("gpu pool budgets = %v/%v, want 0.980/1.0", gpu.BudgetUsed, gpu.BudgetTotal)
	}
	if len(gpu.Classes) != 1 || gpu.Classes[0].LadderClass != "hw" {
		t.Fatalf("gpu pool classes = %+v, want one hw class", gpu.Classes)
	}
	if len(gpu.Classes[0].RungFits) != 1 || gpu.Classes[0].RungFits[0].MaxStreams != 4 {
		t.Errorf("gpu hw rung_fits = %+v, want [maxStreams=4]", gpu.Classes[0].RungFits)
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
