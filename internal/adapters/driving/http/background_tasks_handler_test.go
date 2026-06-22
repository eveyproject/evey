package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/pkg/bgtasks"
)

func TestBackgroundTasksRoute_Admin_ReturnsSnapshot(t *testing.T) {
	bgtasks.Register("test_worker_bgroute")

	mux := http.NewServeMux()
	registerBackgroundTasksRoute(mux, validUserValidator(true))

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
	registerBackgroundTasksRoute(mux, validUserValidator(false))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/background-tasks", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
