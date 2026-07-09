package transcoding

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoVideoSessionCapacity_NoSamples_FailSafeOne(t *testing.T) {
	t.Parallel()
	capacity, evidence := AutoVideoSessionCapacity(nil, 8)
	assert.Equal(t, 1, capacity)
	assert.False(t, evidence.Derived)
	assert.Equal(t, 8, evidence.CPUBudget)
}

func TestAutoVideoSessionCapacity_NoPassingRung_FailSafeOne(t *testing.T) {
	t.Parallel()
	samples := []transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 10, RequiredFPS: 30, Passing: false},
		{Ladder: "software", Height: 1080, FPS: 5, RequiredFPS: 30, Passing: false},
	}
	capacity, evidence := AutoVideoSessionCapacity(samples, 4)
	assert.Equal(t, 1, capacity)
	assert.False(t, evidence.Derived)
}

func TestAutoVideoSessionCapacity_RatioBelowOne_FloorsToOne(t *testing.T) {
	t.Parallel()
	samples := []transcoding.BenchRungSample{
		{Ladder: "software", Height: 1080, FPS: 25, RequiredFPS: 30, Passing: true},
	}
	capacity, evidence := AutoVideoSessionCapacity(samples, 8)
	assert.Equal(t, 1, capacity)
	assert.True(t, evidence.Derived)
	assert.InDelta(t, 25.0/30.0, evidence.Ratio, 0.0001)
}

func TestAutoVideoSessionCapacity_RatioBoundedByCPUBudget(t *testing.T) {
	t.Parallel()
	samples := []transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 300, RequiredFPS: 30, Passing: true},
	}
	capacity, evidence := AutoVideoSessionCapacity(samples, 4)
	assert.Equal(t, 4, capacity, "ratio=10 must clamp to cpuBudget=4")
	assert.True(t, evidence.Derived)
	assert.Equal(t, 4, evidence.CPUBudget)
}

func TestAutoVideoSessionCapacity_PicksTallestPassingSoftwareRung(t *testing.T) {
	t.Parallel()
	samples := []transcoding.BenchRungSample{
		{Ladder: "software", Height: 480, FPS: 90, RequiredFPS: 30, Passing: true},   // ratio 3
		{Ladder: "software", Height: 1080, FPS: 60, RequiredFPS: 30, Passing: true},  // ratio 2, but taller
		{Ladder: "software", Height: 2160, FPS: 10, RequiredFPS: 30, Passing: false}, // failing, ignored
	}
	capacity, evidence := AutoVideoSessionCapacity(samples, 8)
	assert.Equal(t, 1080, evidence.Height)
	assert.Equal(t, 2, capacity)
}

func TestAutoVideoSessionCapacity_IgnoresHWAndHDRRungs(t *testing.T) {
	t.Parallel()
	samples := []transcoding.BenchRungSample{
		{Ladder: "hw", Height: 2160, FPS: 300, RequiredFPS: 30, Passing: true},
		{Ladder: "software-hdr", Height: 1080, FPS: 300, RequiredFPS: 30, Passing: true},
		{Ladder: "software", Height: 720, FPS: 60, RequiredFPS: 30, Passing: true},
	}
	capacity, evidence := AutoVideoSessionCapacity(samples, 8)
	assert.Equal(t, 720, evidence.Height)
	assert.Equal(t, 2, capacity)
}

func TestAutoVideoSessionCapacity_ZeroOrNegativeCPUBudget_ClampsToOne(t *testing.T) {
	t.Parallel()
	_, evidence := AutoVideoSessionCapacity(nil, 0)
	assert.Equal(t, 1, evidence.CPUBudget)
	_, evidence = AutoVideoSessionCapacity(nil, -5)
	assert.Equal(t, 1, evidence.CPUBudget)
}

// mapConfigRepo is a minimal in-memory shared.ServerConfigRepository fake for
// ResolveVideoSessionLimit tests.
type mapConfigRepo struct {
	values map[string]string
}

func (r *mapConfigRepo) Get(_ context.Context, key string) (*shared.ServerConfigEntry, error) {
	v, ok := r.values[key]
	if !ok {
		return nil, nil
	}
	return &shared.ServerConfigEntry{Key: key, Value: v}, nil
}
func (r *mapConfigRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}
func (r *mapConfigRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}
func (r *mapConfigRepo) List(_ context.Context) ([]*shared.ServerConfigEntry, error) {
	return nil, nil
}

func TestResolveVideoSessionLimit_NilRepo_ReturnsAuto(t *testing.T) {
	t.Parallel()
	got := ResolveVideoSessionLimit(context.Background(), nil, 3)
	assert.Equal(t, 3, got)
}

func TestResolveVideoSessionLimit_AbsentKey_ReturnsAuto(t *testing.T) {
	t.Parallel()
	repo := &mapConfigRepo{}
	got := ResolveVideoSessionLimit(context.Background(), repo, 3)
	assert.Equal(t, 3, got)
}

func TestResolveVideoSessionLimit_ExplicitAuto_ReturnsAuto(t *testing.T) {
	t.Parallel()
	repo := &mapConfigRepo{values: map[string]string{MaxVideoSessionsConfigKey: "auto"}}
	got := ResolveVideoSessionLimit(context.Background(), repo, 3)
	assert.Equal(t, 3, got)
}

func TestResolveVideoSessionLimit_ValidReduction_Applied(t *testing.T) {
	t.Parallel()
	repo := &mapConfigRepo{values: map[string]string{MaxVideoSessionsConfigKey: "2"}}
	got := ResolveVideoSessionLimit(context.Background(), repo, 5)
	assert.Equal(t, 2, got)
}

func TestResolveVideoSessionLimit_ValueAboveAuto_ClampedToAuto(t *testing.T) {
	t.Parallel()
	repo := &mapConfigRepo{values: map[string]string{MaxVideoSessionsConfigKey: "999"}}
	got := ResolveVideoSessionLimit(context.Background(), repo, 4)
	assert.Equal(t, 4, got, "admin can only reduce, never exceed the auto capacity")
}

func TestResolveVideoSessionLimit_InvalidValue_FallsBackToAuto(t *testing.T) {
	t.Parallel()
	repo := &mapConfigRepo{values: map[string]string{MaxVideoSessionsConfigKey: "banana"}}
	got := ResolveVideoSessionLimit(context.Background(), repo, 4)
	assert.Equal(t, 4, got)
}

func TestResolveVideoSessionLimit_ZeroOrNegativeValue_FallsBackToAuto(t *testing.T) {
	t.Parallel()
	repo := &mapConfigRepo{values: map[string]string{MaxVideoSessionsConfigKey: "0"}}
	got := ResolveVideoSessionLimit(context.Background(), repo, 4)
	assert.Equal(t, 4, got)

	repo = &mapConfigRepo{values: map[string]string{MaxVideoSessionsConfigKey: "-1"}}
	got = ResolveVideoSessionLimit(context.Background(), repo, 4)
	assert.Equal(t, 4, got)
}

func TestResolveVideoSessionLimit_AutoLessThanOne_ClampedToOne(t *testing.T) {
	t.Parallel()
	got := ResolveVideoSessionLimit(context.Background(), nil, 0)
	require.Equal(t, 1, got)
}

// ============================================================================
// VideoCostModel
// ============================================================================

func TestNewVideoCostModel_NoSamples_ReturnsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, NewVideoCostModel(nil))
}

func TestNewVideoCostModel_NoUsableRung_ReturnsNil(t *testing.T) {
	t.Parallel()
	samples := []transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 82, RequiredFPS: 30, Passing: false},       // failing
		{Ladder: "hw", Height: 1080, FPS: 0, RequiredFPS: 30, Passing: true},              // fps <= 0
		{Ladder: "software-hdr", Height: 1080, FPS: 100, RequiredFPS: 30, Passing: false}, // failing
		{Ladder: "software", Height: 1440, FPS: 0, RequiredFPS: 30, Passing: true},        // fps <= 0
	}
	assert.Nil(t, NewVideoCostModel(samples))
}

// A HW-only usable rung (no software evidence at all) still builds a model -
// classes are independent, so hw evidence is never discarded just because
// software has none. CostFor(LadderClassHW, ...) reads it directly;
// CostFor(LadderClassSoftware, ...) still fails safe to 1.0 (no software
// table and software IS the fallback target, so there is nowhere left to
// fall back to).
func TestNewVideoCostModel_HWOnlyRung_BuildsHWTable(t *testing.T) {
	t.Parallel()
	samples := []transcoding.BenchRungSample{
		{Ladder: "hw", Height: 1080, FPS: 100, RequiredFPS: 30, Passing: true},
	}
	m := NewVideoCostModel(samples)
	require.NotNil(t, m)
	assert.InDelta(t, 30.0/100.0, m.CostFor(LadderClassHW, 1080), 0.0001)
	assert.InDelta(t, 1.0, m.CostFor(LadderClassSoftware, 1080), 0.0001)
}

func TestVideoCostModel_CostForHeight_NilReceiver_FailSafeOne(t *testing.T) {
	t.Parallel()
	var m *VideoCostModel
	assert.InDelta(t, 1.0, m.CostForHeight(1080), 0.0001)
}

func TestVideoCostModel_CostForHeight_IntermediateHeight_UsesNextTallerRung(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 82.0, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1080, FPS: 69.6, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1440, FPS: 56.6, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	// 1000 has no exact rung: must use the smallest rung >= 1000, i.e. 1080.
	assert.InDelta(t, 30.0/69.6, m.CostForHeight(1000), 0.0001)
}

func TestVideoCostModel_CostForHeight_BeyondTallestRung_UsesTallestRung(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 82.0, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1080, FPS: 69.6, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1440, FPS: 56.6, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	// 2160 exceeds every calibrated rung: falls back to the tallest, 1440p.
	assert.InDelta(t, 30.0/56.6, m.CostForHeight(2160), 0.0001)
}

func TestVideoCostModel_CostForHeight_IgnoresHWAndHDRAndZeroFPSSamples(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "hw", Height: 2160, FPS: 300, RequiredFPS: 30, Passing: true},
		{Ladder: "software-hdr", Height: 1080, FPS: 300, RequiredFPS: 30, Passing: true},
		{Ladder: "software", Height: 1440, FPS: 0, RequiredFPS: 30, Passing: true}, // ignored: FPS<=0
		{Ladder: "software", Height: 720, FPS: 82.0, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	// Only the 720p software rung is usable; every height (even far above it)
	// resolves to that rung's cost.
	assert.InDelta(t, 30.0/82.0, m.CostForHeight(2160), 0.0001)
}

func TestVideoCostModel_MaxStreamsAt_FloorsAndMinimumOne(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 82.0, RequiredFPS: 30.0, Passing: true},  // cost ~0.366 -> 2 streams
		{Ladder: "software", Height: 1440, FPS: 56.6, RequiredFPS: 30.0, Passing: true}, // cost ~0.530 -> 1 stream
	})
	require.NotNil(t, m)
	assert.Equal(t, 2, m.MaxStreamsAt(720))
	assert.Equal(t, 1, m.MaxStreamsAt(1440))
}

func TestVideoCostModel_MaxStreamsAt_NilReceiver_ReturnsOne(t *testing.T) {
	t.Parallel()
	var m *VideoCostModel
	assert.Equal(t, 1, m.MaxStreamsAt(1080))
}

func TestVideoCostModel_Rungs_SortedAscendingWithMaxStreams(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 1440, FPS: 56.6, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 720, FPS: 82.0, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1080, FPS: 69.6, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	rungs := m.Rungs()
	require.Len(t, rungs, 3)
	assert.Equal(t, []int{720, 1080, 1440}, []int{rungs[0].Height, rungs[1].Height, rungs[2].Height})
	for _, r := range rungs {
		assert.Equal(t, m.MaxStreamsAt(r.Height), r.MaxStreams)
	}
}

func TestVideoCostModel_Rungs_NilReceiver_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	var m *VideoCostModel
	assert.Empty(t, m.Rungs())
}

func TestTopProfileHeight_EmptySlice_ReturnsZero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, topProfileHeight(nil))
}

func TestTopProfileHeight_PicksTallest(t *testing.T) {
	t.Parallel()
	profiles := []transcoding.HLSProfile{
		{Height: 480},
		{Height: 1440},
		{Height: 720},
	}
	assert.Equal(t, 1440, topProfileHeight(profiles))
}

// ============================================================================
// LadderClass / ResourcePool / LadderClassOf
// ============================================================================

// devBoxSamples mirrors the real measured dev-box calibration referenced
// throughout this ladder-class suite: software 720=0.366/1080=0.431/
// 1440=0.530 ; hw 720=0.663/1080=0.245/1440=0.300/2160=0.465. The hw column
// is deliberately NOT monotonic in height (720p hw costs MORE than 1080p
// hw) - a real measured shape, not an idealisation, and exactly why
// CheapestRung exists instead of always picking the shortest rung.
func devBoxSamples() []transcoding.BenchRungSample {
	const required = 30.0
	rung := func(ladder string, height int, cost float64) transcoding.BenchRungSample {
		return transcoding.BenchRungSample{
			Ladder: ladder, Height: height, RequiredFPS: required, FPS: required / cost, Passing: true,
		}
	}
	return []transcoding.BenchRungSample{
		rung("software", 720, 0.366),
		rung("software", 1080, 0.431),
		rung("software", 1440, 0.530),
		rung("hw", 720, 0.663),
		rung("hw", 1080, 0.245),
		rung("hw", 1440, 0.300),
		rung("hw", 2160, 0.465),
	}
}

func TestLadderClass_Pool(t *testing.T) {
	t.Parallel()
	assert.Equal(t, PoolGPU, LadderClassHW.Pool())
	assert.Equal(t, PoolCPU, LadderClassSoftware.Pool())
	assert.Equal(t, PoolCPU, LadderClassSoftwareHDR.Pool())
	assert.Equal(t, PoolCPU, LadderClassSoftwareAV1.Pool())
}

func TestLadderClassOf_HWAccelWins(t *testing.T) {
	t.Parallel()
	fp := DecisionFingerprint{HWAccel: "nvenc", VideoCodec: "av1", HDRTonemap: true}
	assert.Equal(t, LadderClassHW, LadderClassOf(fp))
}

func TestLadderClassOf_AV1(t *testing.T) {
	t.Parallel()
	assert.Equal(t, LadderClassSoftwareAV1, LadderClassOf(DecisionFingerprint{VideoCodec: "av1"}))
	assert.Equal(t, LadderClassSoftwareAV1, LadderClassOf(DecisionFingerprint{VideoCodec: "AV1"}))
}

func TestLadderClassOf_HDRTonemap(t *testing.T) {
	t.Parallel()
	assert.Equal(t, LadderClassSoftwareHDR, LadderClassOf(DecisionFingerprint{HDRTonemap: true}))
}

func TestLadderClassOf_PlainSoftware(t *testing.T) {
	t.Parallel()
	assert.Equal(t, LadderClassSoftware, LadderClassOf(DecisionFingerprint{VideoCodec: "h264"}))
	assert.Equal(t, LadderClassSoftware, LadderClassOf(DecisionFingerprint{}))
}

// ============================================================================
// VideoCostModel - multi-class (CostFor / CheapestRung / RungsFor / Classes)
// ============================================================================

func TestVideoCostModel_CostFor_PerClassTables(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel(devBoxSamples())
	require.NotNil(t, m)

	assert.InDelta(t, 0.366, m.CostFor(LadderClassSoftware, 720), 0.001)
	assert.InDelta(t, 0.431, m.CostFor(LadderClassSoftware, 1080), 0.001)
	assert.InDelta(t, 0.530, m.CostFor(LadderClassSoftware, 1440), 0.001)

	assert.InDelta(t, 0.663, m.CostFor(LadderClassHW, 720), 0.001)
	assert.InDelta(t, 0.245, m.CostFor(LadderClassHW, 1080), 0.001)
	assert.InDelta(t, 0.300, m.CostFor(LadderClassHW, 1440), 0.001)
	assert.InDelta(t, 0.465, m.CostFor(LadderClassHW, 2160), 0.001)
}

func TestVideoCostModel_CostFor_AbsentClassFallsBackToSoftware(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 1080, FPS: 69.6, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	// No hw table at all: LadderClassHW falls back to the software table.
	assert.InDelta(t, m.CostFor(LadderClassSoftware, 1080), m.CostFor(LadderClassHW, 1080), 0.0001)
}

func TestVideoCostModel_CostFor_NoEvidenceAnywhere_FailSafeOne(t *testing.T) {
	t.Parallel()
	// A model with only an hw table and a request for software: software
	// IS the fallback target for every other class, so once it is itself
	// absent there is nowhere left to fall back to - fail safe 1.0.
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "hw", Height: 1080, FPS: 100, RequiredFPS: 30, Passing: true},
	})
	require.NotNil(t, m)
	assert.InDelta(t, 1.0, m.CostFor(LadderClassSoftware, 1080), 0.0001)
	assert.InDelta(t, 1.0, m.CostFor(LadderClassSoftwareHDR, 1080), 0.0001, "software-hdr has no table either and falls back to the also-absent software table")
}

func TestVideoCostModel_CostFor_NilReceiver_FailSafeOne(t *testing.T) {
	t.Parallel()
	var m *VideoCostModel
	assert.InDelta(t, 1.0, m.CostFor(LadderClassHW, 1080), 0.0001)
}

// The exact scenario the phase exists for: a box with a GPU can be steered
// to its cheapest hw rung, which is NOT its shortest rung.
func TestVideoCostModel_CheapestRung_HWIsNotMonotonic(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel(devBoxSamples())
	require.NotNil(t, m)

	height, cost, ok := m.CheapestRung(LadderClassHW)
	require.True(t, ok)
	assert.Equal(t, 1080, height, "1080p hw (0.245) is cheaper than 720p hw (0.663)")
	assert.InDelta(t, 0.245, cost, 0.001)
}

func TestVideoCostModel_CheapestRung_Software(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel(devBoxSamples())
	require.NotNil(t, m)

	height, cost, ok := m.CheapestRung(LadderClassSoftware)
	require.True(t, ok)
	assert.Equal(t, 720, height)
	assert.InDelta(t, 0.366, cost, 0.001)
}

func TestVideoCostModel_CheapestRung_AbsentClass_NotOK(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 1080, FPS: 69.6, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	_, _, ok := m.CheapestRung(LadderClassHW)
	assert.False(t, ok, "no hw table in this model, and CheapestRung does not fall back to software")
}

func TestVideoCostModel_CheapestRung_NilReceiver_NotOK(t *testing.T) {
	t.Parallel()
	var m *VideoCostModel
	_, _, ok := m.CheapestRung(LadderClassSoftware)
	assert.False(t, ok)
}

func TestVideoCostModel_RungsFor_HWClass(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel(devBoxSamples())
	require.NotNil(t, m)
	rungs := m.RungsFor(LadderClassHW)
	require.Len(t, rungs, 4)
	assert.Equal(t, []int{720, 1080, 1440, 2160}, []int{rungs[0].Height, rungs[1].Height, rungs[2].Height, rungs[3].Height})
}

func TestVideoCostModel_RungsFor_AbsentClass_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 1080, FPS: 69.6, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	assert.Empty(t, m.RungsFor(LadderClassHW))
}

func TestVideoCostModel_Classes_OnlySoftwareOnANonHWBox(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 82.0, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	assert.Equal(t, []LadderClass{LadderClassSoftware}, m.Classes())
}

func TestVideoCostModel_Classes_IncludesHWWhenBenched(t *testing.T) {
	t.Parallel()
	m := NewVideoCostModel(devBoxSamples())
	require.NotNil(t, m)
	assert.Contains(t, m.Classes(), LadderClassHW)
	assert.Contains(t, m.Classes(), LadderClassSoftware)
}

func TestVideoCostModel_Classes_NilReceiver_ReturnsNil(t *testing.T) {
	t.Parallel()
	var m *VideoCostModel
	assert.Nil(t, m.Classes())
}

// No behaviour change on a server with no hw bench samples: CostForHeight
// (the pre-existing, still-supported entry point) is byte-identical to
// before ladder classes existed, and CostFor(LadderClassHW, ...) simply
// falls back to the same software numbers - there is no way for a
// software-only box to observe a different admission outcome.
func TestVideoCostModel_NoHWSamples_BehaviourUnchanged(t *testing.T) {
	t.Parallel()
	samples := []transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 82.0, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1080, FPS: 69.6, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1440, FPS: 56.6, RequiredFPS: 30.0, Passing: true},
	}
	m := NewVideoCostModel(samples)
	require.NotNil(t, m)
	for _, h := range []int{480, 720, 1000, 1080, 1440, 2160} {
		assert.InDelta(t, m.CostForHeight(h), m.CostFor(LadderClassHW, h), 0.0001,
			"hw falls back to the same software cost this box would have charged before ladder classes existed")
	}
	assert.Equal(t, []LadderClass{LadderClassSoftware}, m.Classes(), "no hw evidence -> no hw class surfaced")
}
