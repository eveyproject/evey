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

	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// fakeCapOverrideLister records calls and returns canned data.
type fakeCapOverrideLister struct {
	overrides []domaintranscoding.CapOverride
}

func (f *fakeCapOverrideLister) ListActive() []domaintranscoding.CapOverride {
	return f.overrides
}

// fakeCapOverrideResetter implements capOverrideResetter.
// It records the sourceClass it received and returns a canned result.
type fakeCapOverrideResetter struct {
	resetErr        error
	lastSourceClass string
	called          int
}

func (f *fakeCapOverrideResetter) ResetOverride(_ context.Context, sourceClass string) (bool, error) {
	f.called++
	f.lastSourceClass = sourceClass
	return true, f.resetErr
}

// --- transcodeOverridesGetHandler ---

func TestTranscodeOverridesGet_NilLister_Returns503(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode/overrides", nil)
	w := httptest.NewRecorder()
	transcodeOverridesGetHandler(nil)(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestTranscodeOverridesGet_ReturnsOverrides(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	lister := &fakeCapOverrideLister{
		overrides: []domaintranscoding.CapOverride{
			{
				SourceClass:  "hevc/2160p",
				MaxHeight:    1080,
				Reason:       "sustained starvation",
				StarvedRatio: 0.42,
				SamplesCount: 17,
				AppliedAt:    now,
				ExpiresAt:    now.Add(7 * 24 * time.Hour),
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode/overrides", nil)
	w := httptest.NewRecorder()
	transcodeOverridesGetHandler(lister)(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body struct {
		Overrides []domaintranscoding.CapOverride `json:"overrides"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Overrides, 1)
	o := body.Overrides[0]
	assert.Equal(t, "hevc/2160p", o.SourceClass)
	assert.Equal(t, 1080, o.MaxHeight)
	assert.Equal(t, "sustained starvation", o.Reason)
	assert.InDelta(t, 0.42, o.StarvedRatio, 0.001)
	assert.Equal(t, 17, o.SamplesCount)
}

func TestTranscodeOverridesGet_NilSlice_ReturnsEmptyArray(t *testing.T) {
	// ListActive may return nil when there are no active overrides;
	// the handler normalises it to [] so the JSON client never sees null.
	lister := &fakeCapOverrideLister{overrides: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode/overrides", nil)
	w := httptest.NewRecorder()
	transcodeOverridesGetHandler(lister)(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Overrides []domaintranscoding.CapOverride `json:"overrides"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotNil(t, body.Overrides, "nil slice must be normalised to [] before encoding")
}

// --- transcodeOverrideResetHandler ---

func TestTranscodeOverrideReset_NilSvc_Returns503(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode/overrides?source_class=hevc%2F2160p", nil)
	w := httptest.NewRecorder()
	transcodeOverrideResetHandler(nil)(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestTranscodeOverrideReset_MissingSourceClass_Returns400(t *testing.T) {
	// The handler validates source_class; missing param → 400.
	svc := &fakeCapOverrideResetter{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode/overrides", nil)
	w := httptest.NewRecorder()
	transcodeOverrideResetHandler(svc)(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, 0, svc.called, "ResetOverride must not be called when source_class is missing")
}

func TestTranscodeOverrideReset_ValidSourceClass_Returns204(t *testing.T) {
	svc := &fakeCapOverrideResetter{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode/overrides?source_class=hevc%2F2160p", nil)
	w := httptest.NewRecorder()
	transcodeOverrideResetHandler(svc)(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, 1, svc.called, "ResetOverride must be called exactly once")
	assert.Equal(t, "hevc/2160p", svc.lastSourceClass)
}

func TestTranscodeOverrideReset_ServiceError_Returns500(t *testing.T) {
	svc := &fakeCapOverrideResetter{resetErr: newTestErr("db timeout")}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode/overrides?source_class=h264%2F1080p", nil)
	w := httptest.NewRecorder()
	transcodeOverrideResetHandler(svc)(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, 1, svc.called)
	assert.Equal(t, "h264/1080p", svc.lastSourceClass)
}
