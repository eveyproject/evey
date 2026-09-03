package plugin_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	plugindomain "github.com/evey-project/evey/internal/domain/plugin"
)

// Installing from a URL used to skip the preview: the admin saw the manifest
// and the analysis findings only once the plugin was installed and running.
// PreviewSource downloads what the install would, previews it, and installs
// NOTHING.

func TestPreviewSource_URL_PreviewsWithoutInstalling(t *testing.T) {
	zipContent := makeMinimalPluginZip(t, "url-preview", "1.0.0")
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipContent)
	}))
	defer srv.Close()

	svc, repo, _, parser, _, analyzer := newTestInstallerService()
	parser.Result = goManifest("url-preview")
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:      true,
		FileCount: 2,
		Issues:    []string{"[WARNING] overreach: docker invoked at runtime"},
	}

	preview, err := svc.PreviewSource(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "url-preview", preview.Name)
	require.NotNil(t, preview.Analysis)
	assert.Equal(t, []string{"[WARNING] overreach: docker invoked at runtime"}, preview.Analysis.Issues,
		"the warning the install would only show afterwards is on the preview")
	h := sha256.Sum256(zipContent)
	assert.Equal(t, hex.EncodeToString(h[:]), preview.SourceHash)
	assert.Equal(t, 1, hits)

	all, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, all, "a preview installs nothing")
}

func TestPreviewSource_URL_HashFragmentIsChecked(t *testing.T) {
	zipContent := makeMinimalPluginZip(t, "url-preview-hash", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipContent)
	}))
	defer srv.Close()

	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("url-preview-hash")

	_, err := svc.PreviewSource(context.Background(), srv.URL+"#sha256:"+strings.Repeat("0", 64))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")

	h := sha256.Sum256(zipContent)
	preview, err := svc.PreviewSource(context.Background(), srv.URL+"#sha256:"+hex.EncodeToString(h[:]))
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(h[:]), preview.SourceHash)
}

func TestPreviewSource_LocalPath(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("local-preview")

	preview, err := svc.PreviewSource(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, "local-preview", preview.Name)
}

func TestPreviewSource_URL_DownloadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	svc, _, _, _, _, _ := newTestInstallerService()
	_, err := svc.PreviewSource(context.Background(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve plugin source")
}
