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
func (f *fakeCalibrationSvc) RecentFallbacks() []apptranscoding.HWFallbackEvent {
	return f.fallbacks
}
func (f *fakeCalibrationSvc) Recalibrate(_ context.Context) error {
	f.calls++
	if f.recalErr != nil {
		return f.recalErr
	}
	f.current = domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1440, BenchmarkedAt: time.Now().UTC()}
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
	assert.Equal(t, "abc", got.CPUSignature)
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
