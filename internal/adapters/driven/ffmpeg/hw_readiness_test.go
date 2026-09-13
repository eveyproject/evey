package ffmpeg

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func readinessRunner() *BenchmarkRunner {
	return &BenchmarkRunner{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// withFastRetries keeps the bounded schedule's SHAPE (four attempts) but makes
// each delay 1ms so the tests do not sleep the real 0.5/1.5/3/5s.
func withFastRetries(t *testing.T) {
	t.Helper()
	prev := hwReadinessRetryDelays
	hwReadinessRetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { hwReadinessRetryDelays = prev })
}

const vaapiInitStderr = "Failed to create a VAAPI device"

// A transient init failure that clears on replay certifies the backend: the
// probe is retried and its later success is what the caller sees, so the first
// failure is never persisted as unsupported.
func TestReadinessProbe_TransientInitRecovers(t *testing.T) {
	withFastRetries(t)
	r := readinessRunner()
	calls := 0
	ok := r.runHWReadinessProbe(context.Background(), "encode_init", "vaapi", "/dev/dri/renderD128", ClassifyHWFailure,
		func() (bool, string, string, error) {
			calls++
			if calls == 1 {
				return false, "ffmpeg -vaapi_device ...", vaapiInitStderr, errors.New("exit status 1")
			}
			return true, "ffmpeg -vaapi_device ...", "", nil
		})
	if !ok {
		t.Fatal("a transient init that recovers must certify the backend")
	}
	if calls != 2 {
		t.Fatalf("expected exactly one retry (2 calls), got %d", calls)
	}
}

// A deterministic unsupported-option failure is returned at once, never retried.
func TestReadinessProbe_DeterministicNotRetried(t *testing.T) {
	withFastRetries(t)
	r := readinessRunner()
	calls := 0
	ok := r.runHWReadinessProbe(context.Background(), "encode_init", "vaapi", "", ClassifyHWFailure,
		func() (bool, string, string, error) {
			calls++
			return false, "argv", "Unrecognized option 'zzz'", errors.New("exit status 1")
		})
	if ok {
		t.Fatal("a deterministic unsupported failure must not certify")
	}
	if calls != 1 {
		t.Fatalf("deterministic failure must not be retried, calls=%d", calls)
	}
}

// Retries exhausted stay fail-close: every attempt fails transiently, the probe
// is replayed the full schedule, and the final verdict is not-certified.
func TestReadinessProbe_ExhaustedStaysFailClose(t *testing.T) {
	withFastRetries(t)
	r := readinessRunner()
	calls := 0
	ok := r.runHWReadinessProbe(context.Background(), "encode_init", "vaapi", "", ClassifyHWFailure,
		func() (bool, string, string, error) {
			calls++
			return false, "argv", vaapiInitStderr, errors.New("exit status 1")
		})
	if ok {
		t.Fatal("exhausted retries must stay fail-close")
	}
	if want := len(hwReadinessRetryDelays) + 1; calls != want {
		t.Fatalf("calls=%d, want %d (first + every retry)", calls, want)
	}
}

// The VAAPI->OpenCL derive failure the 2026-09-10 boots hit ends its stderr on
// "Invalid argument": the GENERIC classifier reads that as a deterministic
// route incompatibility (would refuse the retry), which is exactly why the
// OpenCL probe uses a local classifier that calls it an init/readiness failure.
func TestClassifyOpenCLProbe_DeriveIsRetryableInit(t *testing.T) {
	derive := "The hardware pixel format 'opencl' is not supported by the device type 'VAAPI'\n" +
		"Failed to initialise target frames context: -38.\n" +
		"Task finished with error code: -22 (Invalid argument)"
	if got := classifyOpenCLProbe(errors.New("exit status 1"), derive); got != transcoding.FailureHWInit || !got.RetryableInit() {
		t.Fatalf("derive failure must be a retryable hw_init, got %s", got)
	}
	// Precondition that motivates the local classifier: the generic one mislabels it.
	if !ClassifyHWFailure(errors.New("exit status 1"), derive).Deterministic() {
		t.Fatal("precondition: the generic classifier reads the derive tail as deterministic")
	}
	// A named unsupported option is deterministic even for the OpenCL probe.
	if got := classifyOpenCLProbe(errors.New("x"), "Value out of range for option"); got.RetryableInit() {
		t.Fatalf("a named unsupported option must be deterministic, got %s", got)
	}
	if got := classifyOpenCLProbe(nil, ""); got != transcoding.FailureNone {
		t.Fatalf("no error must classify as none, got %s", got)
	}
}

// An OpenCL derive that recovers on replay certifies OpenCL (so the feature is
// added, never persisted as unsupported), using the OpenCL-specific classifier.
func TestReadinessProbe_OpenCLDeriveRecovers(t *testing.T) {
	withFastRetries(t)
	r := readinessRunner()
	calls := 0
	ok := r.runHWReadinessProbe(context.Background(), "opencl_tonemap", "vaapi", "/dev/dri/renderD128", classifyOpenCLProbe,
		func() (bool, string, string, error) {
			calls++
			if calls <= 2 {
				return false, "argv", "not supported by the device type 'VAAPI' -22 (Invalid argument)", errors.New("exit status 1")
			}
			return true, "argv", "", nil
		})
	if !ok {
		t.Fatal("an OpenCL derive that recovers must certify (feature added, not unsupported)")
	}
	if calls != 3 {
		t.Fatalf("expected two retries then success (3 calls), got %d", calls)
	}
}

func TestOpenCLSubstep(t *testing.T) {
	if s := openCLSubstep("not supported by the device type 'VAAPI'"); s != "derive_vaapi_to_opencl" {
		t.Fatalf("derive substep = %q", s)
	}
	if s := openCLSubstep("Failed to create OpenCL device: no platform"); s != "runtime_device_init" {
		t.Fatalf("runtime substep = %q", s)
	}
}
