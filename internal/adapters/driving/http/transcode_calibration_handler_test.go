package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// fakeCalibrationSvc satisfies the calibrationSvc interface for tests.
type fakeCalibrationSvc struct {
	current   domaintranscoding.EncodeCapability
	recalErr  error
	calls     int
	fallbacks []apptranscoding.HWFallbackEvent
	hwMode    domaintranscoding.HWMode
}

func (f *fakeCalibrationSvc) Current() domaintranscoding.EncodeCapability { return f.current }
func (f *fakeCalibrationSvc) HWMode() domaintranscoding.HWMode {
	if f.hwMode == "" {
		return domaintranscoding.HWModeAuto
	}
	return f.hwMode
}
func (f *fakeCalibrationSvc) Status() apptranscoding.CalibrationStatus {
	return apptranscoding.CalibrationStatus{State: "calibrated"}
}
func (f *fakeCalibrationSvc) RecentFallbacks() []apptranscoding.HWFallbackEvent {
	return f.fallbacks
}
func (f *fakeCalibrationSvc) Recalibrate(_ context.Context) error {
	f.calls++
	if f.recalErr != nil {
		return f.recalErr
	}
	// Include host-internal fields so the recalibrate-response redaction test
	// exercises the handler's redaction, not the fake.
	f.current = domaintranscoding.EncodeCapability{
		SoftwareMaxHeight: 1440,
		HWDevice:          "/dev/dri/renderD128",
		CPUSignature:      "xyz",
		BenchmarkedAt:     time.Now().UTC(),
	}
	return nil
}

func TestTranscodeCalibrationGet_NoSvc_Returns503(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode/calibration", nil)
	w := httptest.NewRecorder()
	transcodeCalibrationGetHandler(nil)(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code,
		"missing svc must surface as 503, not 500 (helps the admin distinguish 'feature off' from 'real error')")
}

func TestTranscodeCalibrationGet_ReturnsSnapshot(t *testing.T) {
	svc := &fakeCalibrationSvc{
		current: domaintranscoding.EncodeCapability{
			SoftwareMaxHeight:   1080,
			HWMaxHeight:         2160,
			HWBackend:           "nvenc",
			HWDevice:            "/dev/dri/renderD128",
			CPUSignature:        "abc",
			BenchmarkedAt:       time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC),
			BenchmarkDurationMs: 5200,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode/calibration", nil)
	w := httptest.NewRecorder()
	transcodeCalibrationGetHandler(svc)(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got domaintranscoding.EncodeCapability
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 1080, got.SoftwareMaxHeight)
	assert.Equal(t, 2160, got.HWMaxHeight)
	assert.Equal(t, "nvenc", got.HWBackend)

	// Host-internal fields must be redacted at the API boundary, the rest kept.
	body := w.Body.String()
	assert.NotContains(t, body, "hw_device", "DRM device path must not leak")
	assert.NotContains(t, body, "/dev/dri", "DRM device path must not leak")
	assert.NotContains(t, body, "cpu_signature", "CPU identity hash must not leak")
	assert.Contains(t, body, "software_max_height", "kept fields must still serialize")
	assert.Empty(t, got.HWDevice)
	assert.Empty(t, got.CPUSignature)
}

func TestTranscodeRecalibrate_Success_ReturnsFreshSnapshot(t *testing.T) {
	svc := &fakeCalibrationSvc{
		current: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 720},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcode/calibration/recalibrate", nil)
	w := httptest.NewRecorder()
	transcodeRecalibrateHandler(svc)(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, svc.calls, "Recalibrate must run exactly once")
	var got domaintranscoding.EncodeCapability
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 1440, got.SoftwareMaxHeight, "response must be the post-recalibration snapshot")
	// The recalibrate response must redact host internals just like the GET.
	body := w.Body.String()
	assert.NotContains(t, body, "hw_device", "recalibrate response must not leak the DRM device path")
	assert.NotContains(t, body, "/dev/dri", "recalibrate response must not leak the DRM device path")
	assert.NotContains(t, body, "cpu_signature", "recalibrate response must not leak the CPU hash")
}

func TestTranscodeRecalibrate_Busy_Returns409(t *testing.T) {
	svc := &fakeCalibrationSvc{recalErr: apptranscoding.ErrBenchmarkBusy}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcode/calibration/recalibrate", nil)
	w := httptest.NewRecorder()
	transcodeRecalibrateHandler(svc)(w, req)
	assert.Equal(t, http.StatusConflict, w.Code,
		"concurrent recalibrate must surface as 409 Conflict so the UI shows 'already in progress' without retrying")
}

func TestTranscodeRecalibrate_GenericError_Returns500(t *testing.T) {
	svc := &fakeCalibrationSvc{recalErr: errBenchmarkUnexpected}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/transcode/calibration/recalibrate", nil)
	w := httptest.NewRecorder()
	transcodeRecalibrateHandler(svc)(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// errBenchmarkUnexpected is a test-only sentinel so we exercise the
// "any other error" branch of the handler without coupling to a real
// runner failure path.
var errBenchmarkUnexpected = newTestErr("bench crashed unexpectedly")

type testErr string

func (e testErr) Error() string { return string(e) }
func newTestErr(s string) error { return testErr(s) }

// The admin panel prices every measured route through the SAME model the
// limiter admits with: a route's share of the accelerator is required rate
// over measured rate, a CPU-only route charges no accelerator, and a
// capability without route evidence renders no rows.
func TestRouteCostViews_PricesLikeAdmission(t *testing.T) {
	assert.Nil(t, routeCostViews(domaintranscoding.EncodeCapability{}))

	nvenc := domaintranscoding.RouteCostKeyFor("nvenc", "h264_nvenc", domaintranscoding.RouteCostSDR)
	cpu := domaintranscoding.RouteCostKeyFor("", "libx264", domaintranscoding.RouteCostSDR)
	views := routeCostViews(domaintranscoding.EncodeCapability{RouteSamples: []domaintranscoding.RouteSample{
		{Key: nvenc, Height: 1080, FPS: 96, SourceFPS: 24, CPUCores: 0.5},
		{Key: cpu, Height: 1080, FPS: 48, SourceFPS: 24, CPUCores: 4},
	}})
	require.Len(t, views, 2)
	assert.Equal(t, "nvenc", views[0].Backend)
	assert.Equal(t, "h264_nvenc", views[0].Encoder)
	assert.Equal(t, "sdr", views[0].Route)
	assert.Equal(t, 1080, views[0].Height)
	assert.InDelta(t, 24*domaintranscoding.RouteCostHeadroom/96, views[0].AccelCost, 1e-9, "required over measured fps")
	assert.Greater(t, views[0].CPUCost, 0.0)
	assert.Zero(t, views[1].AccelCost, "a CPU encoder spends no accelerator")
	assert.Greater(t, views[1].CPUCost, views[0].CPUCost, "four cores at half the speed cost more of the CPU pool")
}
