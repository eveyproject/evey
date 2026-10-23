package grpcapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// The URL install path shares the installer with the upload route, which
// already tells the admin WHY an archive was refused. The RPC used to fold
// every refusal into "internal error": a broken manifest, a hash that does
// not match the URL fragment, a source that cannot be fetched and an
// archive the analyzer blocked all read the same. Each is the caller's
// problem and must say so, with the reason.
func TestInstallPlugin_RefusalsAreNotServerFaults(t *testing.T) {
	ctx := context.Background()
	newHandler := func(parser *pluginfakes.FakeManifestParser) *adminPluginHandler {
		return &adminPluginHandler{
			pluginInstaller: appplugin.NewInstallerService(pluginfakes.NewFakePluginRepository(),
				pluginfakes.NewFakePluginRunner(), parser, sharedfakes.NewFakeEventBus(),
				appplugin.InstallerWithHTTPClient(http.DefaultClient)), // the SSRF guard would refuse the loopback test server
			enabled: true,
		}
	}
	zip := filepath.Join(t.TempDir(), "p.zip")
	require.NoError(t, os.WriteFile(zip, []byte("zip"), 0o644))

	parser := pluginfakes.NewFakeManifestParser()
	parser.Err = errors.New("manifest.yaml: name is required")
	_, err := newHandler(parser).InstallPlugin(ctx, &eveyv1.InstallPluginRequest{Source: zip})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "name is required")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/p.zip" {
			_, _ = w.Write([]byte("zip"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	valid := pluginfakes.NewFakeManifestParser()
	valid.Result = &plugindomain.PluginManifest{Name: "themed", Version: "1.0.0", Kind: plugindomain.KindTheme}
	_, err = newHandler(valid).InstallPlugin(ctx, &eveyv1.InstallPluginRequest{Source: srv.URL + "/p.zip#sha256:" + strings.Repeat("0", 64)})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "a hash the fragment pins that the archive does not match")
	assert.Contains(t, status.Convert(err).Message(), "hash mismatch")

	_, err = newHandler(valid).InstallPlugin(ctx, &eveyv1.InstallPluginRequest{Source: srv.URL + "/gone.zip"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "a source that cannot be fetched")
	assert.Contains(t, status.Convert(err).Message(), "404")

	blocked := &appplugin.PluginBlockedError{Issues: []string{"[CRITICAL] main.js:1 - eval"}, Categories: []string{"dynamic-eval"}}
	assert.Equal(t, codes.PermissionDenied, status.Code((&adminPluginHandler{}).installPluginError(ctx, blocked)))
	assert.Contains(t, status.Convert((&adminPluginHandler{}).installPluginError(ctx, blocked)).Message(), "eval")
	assert.Equal(t, codes.PermissionDenied, status.Code((&adminPluginHandler{}).installPluginError(ctx, errors.New("plugin blocked: theme.js contains forbidden pattern: eval"))))
	assert.Equal(t, codes.Internal, status.Code((&adminPluginHandler{}).installPluginError(ctx, errors.New("write plugin dir: disk full"))))
}

// Removing what is already gone (a double click on the row) is NotFound, not
// a logged 500, for a plugin and for a runtime alike.
func TestUninstallAndRemoveRuntime_UnknownIdIsNotFound(t *testing.T) {
	h := newTestPluginHandler()
	ctx := context.Background()
	_, err := h.UninstallPlugin(ctx, &eveyv1.UninstallPluginRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = h.RemovePluginRuntime(ctx, &eveyv1.RemovePluginRuntimeRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}
