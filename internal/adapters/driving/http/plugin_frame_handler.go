package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

// The page plugin UIs and theme scripts run in. It replaces a srcdoc iframe,
// which inherits the APP's CSP: running the inlined bundle there forced
// 'unsafe-inline' into the app's script-src, so one XSS in the app could run
// anything. This page carries its own policy instead:
//
//   - sandbox allow-scripts: an opaque origin even when opened outside an
//     iframe, so a page that window.open()s it and posts code gets no access
//     to the app's storage or cookies;
//   - the loader below is the only inline script, allowed by its hash;
//     the scripts it runs come from the parent (which fetched them with the
//     user's token, so no token rides in any URL) as blob: scripts, in order;
//   - connect-src: the hosts the plugin's manifest allows (external_fetch),
//     passed by the parent and validated here, 'none' otherwise;
//   - frame-ancestors 'self': only the app may embed it.
const pluginFrameLoader = `(function(){if(window.parent===window)return;var done=false;` +
	`window.addEventListener('message',function(e){if(done||e.source!==window.parent)return;var d=e.data;` +
	`if(!d||d.type!=='evey-frame-load'||!Array.isArray(d.scripts))return;done=true;` +
	`if(typeof d.base==='string'){var b=document.createElement('base');b.href=d.base;document.head.appendChild(b);}` +
	`if(typeof d.css==='string'){var s=document.createElement('style');s.textContent=d.css;document.head.appendChild(s);}` +
	`d.scripts.forEach(function(c){if(typeof c!=='string')return;var sc=document.createElement('script');sc.async=false;` +
	`sc.src=URL.createObjectURL(new Blob([c],{type:'text/javascript'}));document.body.appendChild(sc);});});` +
	`window.parent.postMessage({type:'evey-frame-shell'},'*');})();`

const pluginFramePath = "/plugin-frame.html"

var (
	pluginFrameLoaderHash = func() string {
		sum := sha256.Sum256([]byte(pluginFrameLoader))
		return base64.StdEncoding.EncodeToString(sum[:])
	}()
	pluginFrameHTML = []byte(`<!DOCTYPE html><html><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<style>*,*::before,*::after{box-sizing:border-box}html,body{margin:0;padding:0;background:transparent;font-family:system-ui,-apple-system,sans-serif}</style>` +
		`</head><body><script>` + pluginFrameLoader + `</script></body></html>`)
)

const maxFrameConnectHosts = 16

// frameConnectSrc turns ?connect=a.example,http://localhost:8080 into a
// source list (a bare host means https); an entry that is not a plain host
// is dropped, nothing left is 'none'.
func frameConnectSrc(raw string) string {
	var hosts []string
	for h := range strings.SplitSeq(raw, ",") {
		h = strings.ToLower(strings.TrimSpace(h))
		// An optional http(s) scheme, then the manifest's own host grammar.
		bare := strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
		if h == "" || !plugin.ExternalFetchHostRe.MatchString(bare) || len(hosts) == maxFrameConnectHosts {
			continue
		}
		if !strings.Contains(h, "://") {
			h = "https://" + h
		}
		hosts = append(hosts, h)
	}
	if len(hosts) == 0 {
		return "'none'"
	}
	return strings.Join(hosts, " ")
}

func pluginFramePolicy(connect string) string {
	return "sandbox allow-scripts; default-src 'none'; " +
		"script-src 'sha256-" + pluginFrameLoaderHash + "' blob:; " +
		"style-src 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' blob:; " +
		"connect-src " + frameConnectSrc(connect) + "; " +
		"base-uri 'self'; form-action 'none'; frame-ancestors 'self'"
}

func pluginFrameHandler(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	// Set after securityHeadersMiddleware: this page's own policy replaces
	// the app's, and the app (only) may frame it.
	h.Set("Content-Security-Policy", pluginFramePolicy(r.URL.Query().Get("connect")))
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	_, _ = w.Write(pluginFrameHTML)
}
