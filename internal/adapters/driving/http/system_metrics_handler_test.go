package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemMetricsRoute_Admin_ReportsHostProcessAndForks(t *testing.T) {
	mux := http.NewServeMux()
	registerSystemMetricsRoute(mux, validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/metrics", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var m systemMetrics
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m))
	assert.Positive(t, m.Load.CPUs)
	assert.Positive(t, m.Memory.TotalBytes)
	assert.Positive(t, m.Process.Goroutines)
	assert.Positive(t, m.Process.HeapSys)
	assert.NotNil(t, m.Forks.ByClass, "an empty group list must serialise as [], not null")
	assert.NotNil(t, m.Forks.Children)
}

func TestSystemMetricsRoute_NonAdmin_Forbidden(t *testing.T) {
	mux := http.NewServeMux()
	registerSystemMetricsRoute(mux, validUserValidator(false))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/metrics", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
