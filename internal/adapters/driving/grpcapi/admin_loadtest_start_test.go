package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"
	"github.com/eveyproject/evey/internal/domain/user"
)

type fakeLoadTestRunner struct {
	startErr error
	started  []apploadtest.Config
	canceled int
	status   apploadtest.Status
}

func (f *fakeLoadTestRunner) Start(_ context.Context, cfg apploadtest.Config) error {
	f.started = append(f.started, cfg)
	return f.startErr
}
func (f *fakeLoadTestRunner) Status() apploadtest.Status { return f.status }
func (f *fakeLoadTestRunner) Cancel()                    { f.canceled++ }

type fakeLoadTestMinter struct {
	err    error
	claims user.Claims
	ttl    time.Duration
}

func (f *fakeLoadTestMinter) GenerateAccessTokenWithTTL(c user.Claims, ttl time.Duration) (string, error) {
	f.claims, f.ttl = c, ttl
	if f.err != nil {
		return "", f.err
	}
	return "minted-token", nil
}

type benchmarkingCalibration struct{ fakeCalibrationSnapshot }

func (benchmarkingCalibration) IsBenchmarking() bool { return true }

func newLoadTestHandler(runner *fakeLoadTestRunner, minter *fakeLoadTestMinter) *adminSystemHandler {
	d := &adminSystemHandler{
		loadTest:          runner,
		loadTestBaseURL:   "http://127.0.0.1:8080",
		loadTestPublicURL: "https://evey.example",
	}
	if minter != nil {
		d.loadTestToken = minter
	}
	return d
}

func TestStartLoadTest_NotWired(t *testing.T) {
	d := &adminSystemHandler{}
	for name, call := range map[string]func() error{
		"start": func() error {
			_, err := d.StartLoadTest(ctxWithClaims("admin", "p", true), &eveyv1.StartLoadTestRequest{})
			return err
		},
		"status": func() error {
			_, err := d.GetLoadTestStatus(context.Background(), &eveyv1.GetLoadTestStatusRequest{})
			return err
		},
		"cancel": func() error {
			_, err := d.CancelLoadTest(context.Background(), &eveyv1.CancelLoadTestRequest{})
			return err
		},
	} {
		assert.Equal(t, codes.Unimplemented, status.Code(call()), name)
	}
}

func TestStartLoadTest_RefusedWhileCalibrating(t *testing.T) {
	runner := &fakeLoadTestRunner{}
	d := newLoadTestHandler(runner, &fakeLoadTestMinter{})
	d.calibrationSvc = benchmarkingCalibration{}

	_, err := d.StartLoadTest(ctxWithClaims("admin", "p", true), &eveyv1.StartLoadTestRequest{})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Empty(t, runner.started, "a calibration bench and a load test must never overlap")
}

func TestStartLoadTest_MintsLongLivedCredentialWithCallerClaims(t *testing.T) {
	runner := &fakeLoadTestRunner{status: apploadtest.Status{Running: true, Phase: "ramp", Viewers: 3}}
	minter := &fakeLoadTestMinter{}
	d := newLoadTestHandler(runner, minter)
	d.calibrationSvc = fakeCalibrationSnapshot{}

	got, err := d.StartLoadTest(ctxWithClaims("admin-7", "prof-1", true), &eveyv1.StartLoadTestRequest{
		Scenario: "natural", MaxViewers: 8, Mode: "seek", DurationSec: 120, ColdCache: true,
	})
	require.NoError(t, err)
	assert.True(t, got.GetRunning())
	assert.Equal(t, "ramp", got.GetPhase())
	assert.EqualValues(t, 3, got.GetViewers())

	require.Len(t, runner.started, 1)
	cfg := runner.started[0]
	assert.Equal(t, "minted-token", cfg.Token, "the run must not borrow the caller's short-lived bearer")
	assert.Equal(t, "http://127.0.0.1:8080", cfg.BaseURL)
	assert.Equal(t, "https://evey.example", cfg.PublicBaseURL)
	assert.Equal(t, "natural", cfg.Scenario)
	assert.Equal(t, 8, cfg.MaxViewers)
	assert.Equal(t, "seek", cfg.Mode)
	assert.Equal(t, 2*time.Minute, cfg.MaxDuration)
	assert.True(t, cfg.ColdCache)
	assert.True(t, cfg.ProfileHarnessCPU)

	assert.Equal(t, "admin-7", minter.claims.UserID, "same identity as the admin who pressed the button")
	assert.True(t, minter.claims.IsAdmin)
	assert.Equal(t, 2*time.Minute+loadTestTokenSlack, minter.ttl, "credential outlives the run by the slack")
}

func TestStartLoadTest_DefaultDurationMintsForTheLongestRun(t *testing.T) {
	runner := &fakeLoadTestRunner{}
	minter := &fakeLoadTestMinter{}
	d := newLoadTestHandler(runner, minter)

	_, err := d.StartLoadTest(ctxWithClaims("admin", "p", true), &eveyv1.StartLoadTestRequest{})
	require.NoError(t, err)
	assert.Equal(t, apploadtest.DefaultEnduranceMaxDuration+loadTestTokenSlack, minter.ttl)
	assert.Zero(t, runner.started[0].MaxDuration, "0 leaves the mode default to the runner")
}

func TestStartLoadTest_MintFailureIsInternal(t *testing.T) {
	runner := &fakeLoadTestRunner{}
	d := newLoadTestHandler(runner, &fakeLoadTestMinter{err: errors.New("keys not loaded")})

	_, err := d.StartLoadTest(ctxWithClaims("admin", "p", true), &eveyv1.StartLoadTestRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.NotContains(t, err.Error(), "keys not loaded", "internal detail stays out of the wire")
	assert.Empty(t, runner.started)
}

func TestStartLoadTest_NoMinterFallsBackToCallerBearer(t *testing.T) {
	runner := &fakeLoadTestRunner{}
	d := newLoadTestHandler(runner, nil)

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer caller.jwt"))
	_, err := d.StartLoadTest(ctx, &eveyv1.StartLoadTestRequest{})
	require.NoError(t, err)
	require.Len(t, runner.started, 1)
	assert.Equal(t, "caller.jwt", runner.started[0].Token)

	_, err = d.StartLoadTest(context.Background(), &eveyv1.StartLoadTestRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "no minter and no bearer = nothing to run as")
	assert.Len(t, runner.started, 1)
}

func TestStartLoadTest_RunnerErrorsMapToStatusCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want codes.Code
	}{
		{"real viewers streaming", apploadtest.ErrStreamsActive, codes.FailedPrecondition},
		{"already running", apploadtest.ErrAlreadyRunning, codes.AlreadyExists},
		{"unknown mode", fmt.Errorf("%w: %q", apploadtest.ErrUnknownMode, "x"), codes.InvalidArgument},
		{"invalid client mix", fmt.Errorf("%w: unknown profile", apploadtest.ErrUnknownScenario), codes.InvalidArgument},
		{"anything else", errors.New("boom"), codes.Internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newLoadTestHandler(&fakeLoadTestRunner{startErr: tc.err}, &fakeLoadTestMinter{})
			_, err := d.StartLoadTest(ctxWithClaims("admin", "p", true), &eveyv1.StartLoadTestRequest{})
			assert.Equal(t, tc.want, status.Code(err))
		})
	}
}

func TestLoadTestStatusAndCancel(t *testing.T) {
	runner := &fakeLoadTestRunner{status: apploadtest.Status{Running: true, Phase: "hold", Viewers: 12, Elapsed: 90 * time.Second}}
	d := newLoadTestHandler(runner, nil)

	got, err := d.GetLoadTestStatus(context.Background(), &eveyv1.GetLoadTestStatusRequest{})
	require.NoError(t, err)
	assert.EqualValues(t, 12, got.GetViewers())
	assert.EqualValues(t, 90_000, got.GetElapsedMs())
	assert.Zero(t, runner.canceled)

	runner.status = apploadtest.Status{Phase: "cancelled"}
	got, err = d.CancelLoadTest(context.Background(), &eveyv1.CancelLoadTestRequest{})
	require.NoError(t, err)
	assert.Equal(t, 1, runner.canceled)
	assert.False(t, got.GetRunning())
	assert.Equal(t, "cancelled", got.GetPhase())
}

// The run gets the loopback and the public address to probe whatever the
// request says: the legacy direct and via_proxy flags change nothing, and a
// replayed seed passes through.
func TestStartLoadTest_PathIsNotTheOperatorsChoice(t *testing.T) {
	runner := &fakeLoadTestRunner{}
	d := newLoadTestHandler(runner, &fakeLoadTestMinter{})
	d.calibrationSvc = fakeCalibrationSnapshot{}

	_, err := d.StartLoadTest(ctxWithClaims("admin-7", "prof-1", true), &eveyv1.StartLoadTestRequest{Scenario: "mkvgo", Direct: true}) //nolint:staticcheck // the legacy flag must stay inert
	require.NoError(t, err)
	_, err = d.StartLoadTest(ctxWithClaims("admin-7", "prof-1", true), &eveyv1.StartLoadTestRequest{Scenario: "mkvgo", ViaProxy: true, Seed: 1792073232940793883}) //nolint:staticcheck // the legacy flag must stay inert
	require.NoError(t, err)
	require.Len(t, runner.started, 2)
	for i, cfg := range runner.started {
		assert.Equal(t, "http://127.0.0.1:8080", cfg.BaseURL, "run %d", i)
		assert.NotEmpty(t, cfg.PublicBaseURL, "run %d keeps the public address to probe", i)
	}
	assert.Equal(t, uint64(1792073232940793883), runner.started[1].Seed, "the seed replays a draw")
}
