package httpapi

import (
	"errors"
	"io/fs"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The mobile and TV apps load their JavaScript from the instance they are
// connected to, so a client runs the bundle that was built with its server.
// The image ships those bundles already signed by the project key: this
// handler only hands bytes out, it holds no key and builds no manifest. An
// instance that serves something else gets it refused by the app, which
// checks the signature against the certificate compiled into it.
//
// Layout under the directory, one bundle per app, platform and native runtime:
//
//	<app>/<platform>/<runtime>/manifest.json   the signed bytes, served as is
//	<app>/<platform>/<runtime>/manifest.sig    the expo-signature header value
//	<app>/<platform>/<runtime>/assets/<name>   content-addressed files
//
// The manifest names its assets by relative path, resolved by the app against
// the manifest URL: the same signed file is valid on every instance.

var (
	clientUpdateApps      = map[string]bool{"mobile": true, "tv": true}
	clientUpdatePlatforms = map[string]bool{"android": true, "ios": true}
	clientUpdateRuntimeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	clientUpdateAssetRe   = regexp.MustCompile(`^[a-f0-9]{64}(\.[a-z0-9]{1,8})?$`)
	clientUpdateSigRe     = regexp.MustCompile(`^[\x20-\x7e]{1,1000}$`)
)

// registerClientUpdateRoutes mounts the Expo Updates protocol (version 1):
//
//	GET /api/v1/client-updates/{app}/manifest
//	GET /api/v1/client-updates/{app}/assets/{platform}/{runtime}/{name}
//
// Public on purpose: the app asks before anyone has signed in, and the bundles
// are the published client code.
func registerClientUpdateRoutes(mux *http.ServeMux, dir string) {
	mux.HandleFunc("GET /api/v1/client-updates/{app}/manifest", clientUpdateManifestHandler(dir))
	mux.HandleFunc("GET /api/v1/client-updates/{app}/assets/{platform}/{runtime}/{name}", clientUpdateAssetHandler(dir))
}

func setClientUpdateProtocolHeaders(h http.Header) {
	h.Set("expo-protocol-version", "1")
	h.Set("expo-sfv-version", "0")
	h.Set("Cache-Control", "private, max-age=0")
}

// clientUpdateBundleDir resolves the bundle directory for a request, or "" when
// any part of the request is not a name this handler serves.
func clientUpdateBundleDir(dir, app, platform, runtime string) string {
	if dir == "" || !clientUpdateApps[app] || !clientUpdatePlatforms[platform] || !clientUpdateRuntimeRe.MatchString(runtime) {
		return ""
	}
	return filepath.Join(dir, app, platform, runtime)
}

func clientUpdateManifestHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setClientUpdateProtocolHeaders(w.Header())
		bundleDir := clientUpdateBundleDir(dir, r.PathValue("app"), r.Header.Get("expo-platform"), r.Header.Get("expo-runtime-version"))
		if bundleDir == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		manifest, err := os.ReadFile(filepath.Join(bundleDir, "manifest.json"))
		if errors.Is(err, fs.ErrNotExist) {
			// No bundle for this native runtime: the app keeps what it runs.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			logInternal(r.Context(), "client-updates: read manifest", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rawSig, err := os.ReadFile(filepath.Join(bundleDir, "manifest.sig"))
		signature := strings.TrimSpace(string(rawSig))
		if err != nil || !clientUpdateSigRe.MatchString(signature) {
			// A bundle without its signature would be refused by every app:
			// say it once here instead of in each client's log.
			slog.WarnContext(r.Context(), "client-updates: bundle has no usable signature", slog.String("dir", bundleDir))
			w.WriteHeader(http.StatusNoContent)
			return
		}

		mw := multipart.NewWriter(w)
		w.Header().Set("Content-Type", "multipart/mixed; boundary="+mw.Boundary())
		part, err := mw.CreatePart(textproto.MIMEHeader{
			"content-disposition": {`form-data; name="manifest"`},
			"content-type":        {"application/json; charset=utf-8"},
			"expo-signature":      {signature},
		})
		if err == nil {
			_, err = part.Write(manifest)
		}
		if err == nil {
			part, err = mw.CreatePart(textproto.MIMEHeader{
				"content-disposition": {`form-data; name="extensions"`},
				"content-type":        {"application/json"},
			})
		}
		if err == nil {
			_, err = part.Write([]byte(`{"assetRequestHeaders":{}}`))
		}
		if err == nil {
			err = mw.Close()
		}
		if err != nil {
			slog.DebugContext(r.Context(), "client-updates: manifest write interrupted", slog.Any("err", err))
		}
	}
}

func clientUpdateAssetHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		bundleDir := clientUpdateBundleDir(dir, r.PathValue("app"), r.PathValue("platform"), r.PathValue("runtime"))
		if bundleDir == "" || !clientUpdateAssetRe.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(bundleDir, "assets", name))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = f.Close() }()
		info, err := f.Stat()
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", clientUpdateAssetContentType(name))
		// The name is the content hash: a given URL never changes meaning.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeContent(w, r, "", info.ModTime(), f)
	}
}

func clientUpdateAssetContentType(name string) string {
	ext := filepath.Ext(name)
	switch ext {
	case ".hbc", ".bundle", ".js":
		return "application/javascript"
	case "":
		return "application/octet-stream"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}
