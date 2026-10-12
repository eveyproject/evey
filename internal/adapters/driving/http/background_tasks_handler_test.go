package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/pkg/bgtasks"
)

func TestBackgroundTasksRoute_Admin_ReturnsSnapshot(t *testing.T) {
	bgtasks.Register("test_worker_bgroute")

	mux := http.NewServeMux()
	registerBackgroundTasksRoute(mux, validUserValidator(true), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/background-tasks", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var tasks []bgtasks.Task
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tasks))
	found := false
	for _, tk := range tasks {
		if tk.Name == "test_worker_bgroute" {
			found = true
		}
	}
	assert.True(t, found, "a registered worker must appear in the snapshot")
}

func TestBackgroundTasksRoute_NonAdmin_Forbidden(t *testing.T) {
	mux := http.NewServeMux()
	registerBackgroundTasksRoute(mux, validUserValidator(false), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/background-tasks", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestBackgroundTasksRoute_StopAndResume(t *testing.T) {
	bgtasks.RegisterControllable("test_worker_ctl")
	bgtasks.Register("test_worker_plain")
	mux := http.NewServeMux()
	registerBackgroundTasksRoute(mux, validUserValidator(true), nil)
	post := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	assert.Equal(t, http.StatusNotFound, post("/api/v1/admin/background-tasks/does_not_exist/stop"))
	assert.Equal(t, http.StatusConflict, post("/api/v1/admin/background-tasks/test_worker_plain/stop"),
		"a worker that does not honour Stop must say so, not pretend")

	// The item in flight is cancelled and the worker reads as paused.
	ctx, done := bgtasks.BeginCtx(context.Background(), "test_worker_ctl", "m1")
	defer done()
	require.Equal(t, http.StatusNoContent, post("/api/v1/admin/background-tasks/test_worker_ctl/stop"))
	require.ErrorIs(t, ctx.Err(), context.Canceled)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/background-tasks", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var views []backgroundTaskView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &views))
	var ctl *backgroundTaskView
	for i := range views {
		if views[i].Name == "test_worker_ctl" {
			ctl = &views[i]
		}
	}
	require.NotNil(t, ctl)
	assert.True(t, ctl.Paused)
	assert.True(t, ctl.Controllable)
	assert.Equal(t, 0, ctl.Processes, "no fork runs for a test worker")

	require.Equal(t, http.StatusNoContent, post("/api/v1/admin/background-tasks/test_worker_ctl/resume"))
	assert.False(t, bgtasks.Paused("test_worker_ctl"))
}

func TestBackgroundTasksRoute_NonAdminCannotStop(t *testing.T) {
	bgtasks.RegisterControllable("test_worker_ctl2")
	mux := http.NewServeMux()
	registerBackgroundTasksRoute(mux, validUserValidator(false), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/background-tasks/test_worker_ctl2/stop", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.False(t, bgtasks.Paused("test_worker_ctl2"))
}
