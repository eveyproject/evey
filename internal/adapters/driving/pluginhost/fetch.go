package pluginhost

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	pluginv1 "github.com/eveyproject/evey/gen/plugin/v1"
	"github.com/eveyproject/evey/internal/pkg/ssrf"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// maxFetchBytes caps a brokered response body. Generous enough for the
	// images/JSON plugins legitimately fetch (weather-v2's full-disc sun image
	// is ~200 KB), small enough that a plugin can't use the host to pull huge
	// payloads into memory.
	maxFetchBytes = 8 << 20 // 8 MiB
	// fetchTimeout bounds the whole brokered request so a slow-loris origin
	// can't pin a host goroutine.
	fetchTimeout = 20 * time.Second
	// maxFetchHeaderBytes caps the total size of request headers a plugin may
	// set, so it can't smuggle a huge payload out via header values.
	maxFetchHeaderBytes = 8 << 10 // 8 KiB
	// maxFetchRedirects bounds the redirect chain the broker will follow. Each
	// hop is re-checked against the plugin's allow-list; the cap stops a
	// redirect loop from pinning a goroutine even within allow-listed hosts.
	maxFetchRedirects = 5
)

// errFetchRedirectOffList is returned by the redirect guard when a hop targets
// a host outside the plugin's allow-list. Wrapped in a *url.Error by the http
// client; detected with errors.Is.
var errFetchRedirectOffList = errors.New("redirect target not in external_fetch allow-list")

// errFetchClient is overridable in tests so the allow-list / validation logic
// can be exercised without real network I/O. Production leaves it nil, which
// selects the SSRF-safe client. NoProxy: the broker fetches untrusted,
// plugin-supplied URLs, so it must never route through an operator's
// HTTP(S)_PROXY - that would ship the real destination to the proxy and let a
// private/metadata target slip past the IP filter.
var newFetchClient = func() *http.Client { return ssrf.SafeHTTPClientNoProxy(fetchTimeout) }

// hostAllowed reports whether host (already lower-cased, no port) is covered by
// the plugin's external_fetch allow-list. An entry may be a bare host
// ("api.open-meteo.com") matched case-insensitively and exactly, or a wildcard
// ("*.example.com") matching any single-or-deeper subdomain but NOT the apex.
// Ports in an allow-list entry are ignored for matching (the SSRF dialer and
// the scheme check bound the reachable ports).
func hostAllowed(host string, allowList []string) bool {
	if host == "" {
		return false
	}
	for _, raw := range allowList {
		entry := strings.ToLower(strings.TrimSpace(raw))
		if entry == "" {
			continue
		}
		// Drop a ":port" suffix if present so "example.com:8443" still matches
		// on host. Bracketed IPv6 literals have no bare-port ambiguity here.
		if i := strings.LastIndex(entry, ":"); i > 0 && !strings.Contains(entry[i:], "]") {
			if _, err := netSplitPort(entry); err == nil {
				entry = entry[:i]
			}
		}
		if strings.HasPrefix(entry, "*.") {
			suffix := entry[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
			continue
		}
		if host == entry {
			return true
		}
	}
	return false
}

// netSplitPort reports an error unless entry ends in a numeric ":port".
func netSplitPort(entry string) (string, error) {
	i := strings.LastIndex(entry, ":")
	if i < 0 || i == len(entry)-1 {
		return "", errors.New("no port")
	}
	for _, r := range entry[i+1:] {
		if r < '0' || r > '9' {
			return "", errors.New("non-numeric port")
		}
	}
	return entry[i+1:], nil
}

// Fetch performs an outbound HTTP(S) request on behalf of an authenticated
// plugin. Direct plugin egress is cut at the sandbox, so this is the ONLY way
// out: the host validates the target against the plugin's external_fetch
// allow-list and dials through an SSRF-safe client (private/reserved IPs are
// rejected after DNS resolution, at connect time). Redirects are re-validated
// against BOTH the IP (dialer) and the allow-listed HOST (CheckRedirect), so a
// 302 to an off-list or internal target is refused. The host attaches none of
// its own credentials (no JWT, cookies, or internal tokens) so the plugin can
// never borrow the server's authority.
func (h *HostServer) Fetch(ctx context.Context, req *pluginv1.FetchRequest) (*pluginv1.FetchResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.Fetch")
	defer span.End()

	pluginID := pluginIDFromCtx(ctx)
	if pluginID == "" {
		return nil, status.Error(codes.Unauthenticated, "plugin not authenticated")
	}

	u, err := url.Parse(strings.TrimSpace(req.Url))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid url")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, status.Error(codes.InvalidArgument, "only http and https are allowed")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil, status.Error(codes.InvalidArgument, "empty host")
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return nil, status.Error(codes.InvalidArgument, "only GET and HEAD are allowed")
	}

	p, err := h.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "plugin %s not found", pluginID)
	}
	if !hostAllowed(host, p.ExternalFetch) {
		// Indistinguishable-ish from a genuine denial; the plugin declared its
		// hosts at install, so this only trips on an off-list target.
		return nil, status.Errorf(codes.PermissionDenied, "host %q not in external_fetch allow-list", host)
	}

	var headerBytes int
	for k, v := range req.Headers {
		headerBytes += len(k) + len(v)
	}
	if headerBytes > maxFetchHeaderBytes {
		return nil, status.Error(codes.InvalidArgument, "request headers too large")
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "cannot build request")
	}
	for k, v := range req.Headers {
		// Never let a plugin set Host via header injection; hop-by-hop and auth
		// headers from the plugin are its own business (no ambient host creds).
		if strings.EqualFold(k, "Host") {
			continue
		}
		httpReq.Header.Set(k, v)
	}

	// Re-apply the allow-list on EVERY redirect hop. The SSRF dialer re-checks
	// the resolved IP on each connect (so the internal network stays out of
	// reach) but never re-checks the HOST, so without this a 302 from an
	// allow-listed open-redirect to an arbitrary external host would let a
	// plugin exfiltrate anywhere - defeating the whole point of the allow-list.
	client := newFetchClient()
	guarded := *client
	allow := p.ExternalFetch
	guarded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxFetchRedirects {
			return errors.New("stopped after too many redirects")
		}
		if !hostAllowed(strings.ToLower(req.URL.Hostname()), allow) {
			return errFetchRedirectOffList
		}
		return nil
	}

	resp, err := guarded.Do(httpReq)
	if err != nil {
		if errors.Is(err, errFetchRedirectOffList) {
			return nil, status.Error(codes.PermissionDenied, "redirect target not in external_fetch allow-list")
		}
		// The SSRF dialer rejects private/reserved IPs here; surface a clean
		// error without leaking internal addresses.
		if errors.Is(err, ssrf.ErrBlockedPrivateIP) {
			return nil, status.Error(codes.PermissionDenied, "target resolves to a blocked address")
		}
		return nil, status.Errorf(codes.Unavailable, "fetch failed: %v", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxFetchBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "read body failed: %v", err)
	}
	truncated := false
	if len(body) > maxFetchBytes {
		body = body[:maxFetchBytes]
		truncated = true
	}

	respHeaders := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	return &pluginv1.FetchResponse{
		StatusCode: int32(resp.StatusCode),
		Headers:    respHeaders,
		Body:       body,
		Truncated:  truncated,
	}, nil
}
