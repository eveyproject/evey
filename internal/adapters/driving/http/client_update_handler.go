package httpapi

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"sync"
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
//	<app>/<platform>/<runtime>/manifest.signed   signature line, certificate line, then the signed bytes
//	<app>/<platform>/<runtime>/assets/<name>     content-addressed files
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

// A manifest lists a few dozen assets: far below this, which only bounds what
// one request can make the server read.
const clientUpdateManifestMaxBytes = 4 << 20

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
	// Not stored at all: an app whose clock was set back would otherwise find
	// a cached answer still fresh and never ask the server.
	h.Set("Cache-Control", "no-store")
}

// clientUpdateBundlePath is the bundle directory of a request, relative to the
// served directory, or "" when any part is not a name this handler serves.
func clientUpdateBundlePath(app, platform, runtime string) string {
	if !clientUpdateApps[app] || !clientUpdatePlatforms[platform] || !clientUpdateRuntimeRe.MatchString(runtime) {
		return ""
	}
	return filepath.Join(app, platform, runtime)
}

// openClientUpdateFile opens rel inside dir and nowhere else: a symbolic link
// planted in the directory cannot make the handler serve a file outside it.
func openClientUpdateFile(dir, rel string) (*os.File, error) {
	if dir == "" {
		return nil, fs.ErrNotExist
	}
	return os.OpenInRoot(dir, rel)
}

// clientUpdateUnsignedLogged remembers the bundles already reported as
// unusable: the route is public, and a warning per request would let anyone
// fill the log.
var clientUpdateUnsignedLogged sync.Map

func clientUpdateManifestHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setClientUpdateProtocolHeaders(w.Header())
		// The answer depends on these two request headers, not on the URL alone.
		w.Header().Set("Vary", "expo-platform, expo-runtime-version")
		bundle := clientUpdateBundlePath(r.PathValue("app"), r.Header.Get("expo-platform"), r.Header.Get("expo-runtime-version"))
		if bundle == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Signature and manifest travel in one file, renamed into place by the
		// build: there is no moment where one belongs to another release.
		f, err := openClientUpdateFile(dir, filepath.Join(bundle, "manifest.signed"))
		if err != nil {
			// No signed bundle for this native runtime: the app keeps what it runs.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(f, clientUpdateManifestMaxBytes+1))
		_ = f.Close()
		if err != nil {
			logInternal(r.Context(), "client-updates: read manifest", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Line one is the signature header, line two the certificate of the key
		// that signed, in base64; the apps check it against the root they carry.
		signature, rest, _ := bytes.Cut(raw, []byte("\n"))
		encodedChain, manifest, found := bytes.Cut(rest, []byte("\n"))
		chain, chainErr := base64.StdEncoding.DecodeString(string(encodedChain))
		// A truncated manifest would fail the signature check on every device
		// with nothing here to say why.
		if len(raw) > clientUpdateManifestMaxBytes || !found || len(manifest) == 0 || !clientUpdateSigRe.Match(signature) || chainErr != nil || len(chain) == 0 {
			if _, seen := clientUpdateUnsignedLogged.LoadOrStore(bundle, true); !seen {
				slog.WarnContext(r.Context(), "client-updates: bundle is unsigned, malformed or too large, not served", slog.String("bundle", bundle))
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		mw := multipart.NewWriter(w)
		w.Header().Set("Content-Type", "multipart/mixed; boundary="+mw.Boundary())
		part, err := mw.CreatePart(textproto.MIMEHeader{
			"content-disposition": {`form-data; name="manifest"`},
			"content-type":        {"application/json; charset=utf-8"},
			"expo-signature":      {string(signature)},
		})
		if err == nil {
			_, err = part.Write(manifest)
		}
		if err == nil {
			part, err = mw.CreatePart(textproto.MIMEHeader{
				"content-disposition": {`form-data; name="certificate_chain"`},
				"content-type":        {"application/x-pem-file"},
			})
		}
		if err == nil {
			_, err = part.Write(chain)
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
		bundle := clientUpdateBundlePath(r.PathValue("app"), r.PathValue("platform"), r.PathValue("runtime"))
		if bundle == "" || !clientUpdateAssetRe.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		f, err := openClientUpdateFile(dir, filepath.Join(bundle, "assets", name))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = f.Close() }()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
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
