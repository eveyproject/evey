package pluginhost

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pluginv1 "github.com/eveyproject/evey/gen/plugin/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestHostAllowed(t *testing.T) {
	tests := []struct {
		name  string
		host  string
		allow []string
		want  bool
	}{
		{"exact match", "api.open-meteo.com", []string{"api.open-meteo.com"}, true},
		{"case insensitive", "API.Open-Meteo.COM", []string{"api.open-meteo.com"}, true},
		{"not listed", "evil.com", []string{"api.open-meteo.com"}, false},
		{"empty host", "", []string{"api.open-meteo.com"}, false},
		{"empty allow-list", "api.open-meteo.com", nil, false},
		{"wildcard subdomain", "tiles.maps.example.com", []string{"*.example.com"}, true},
		{"wildcard does not match apex", "example.com", []string{"*.example.com"}, false},
		{"wildcard does not match suffix trick", "notexample.com", []string{"*.example.com"}, false},
		{"entry with port ignored", "sdo.gsfc.nasa.gov", []string{"sdo.gsfc.nasa.gov:443"}, true},
		{"multiple entries", "b.com", []string{"a.com", "b.com"}, true},
		{"blank entry skipped", "b.com", []string{"", "b.com"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostAllowed(strings.ToLower(tc.host), tc.allow); got != tc.want {
				t.Fatalf("hostAllowed(%q, %v) = %v, want %v", tc.host, tc.allow, got, tc.want)
			}
		})
	}
}

// fakePluginStore returns a single plugin with a configurable allow-list.
type fakePluginStore struct {
	plugin *plugindomain.Plugin
	err    error
}

func (f *fakePluginStore) GetByID(_ context.Context, id string) (*plugindomain.Plugin, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.plugin, nil
}
func (f *fakePluginStore) UpdateConfig(_ context.Context, _ string, config map[string]any) error {
	if f.plugin != nil {
		f.plugin.Config = config
	}
	return nil
}

var _ MediaItemQuerier = (*noopMediaQuerier)(nil)

type noopMediaQuerier struct{}

func (noopMediaQuerier) GetByID(context.Context, shared.MediaItemID) (*library.MediaItem, error) {
	return nil, nil
}
func (noopMediaQuerier) List(context.Context, library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return nil, nil
}
func (noopMediaQuerier) Count(context.Context, library.ListMediaItemsFilter) (int, error) {
	return 0, nil
}

func newFetchTestServer(t *testing.T, allow []string) *HostServer {
	t.Helper()
	h := NewHostServer(noopMediaQuerier{}, &fakePluginStore{
		plugin: &plugindomain.Plugin{ID: "plg-weather", ExternalFetch: allow},
	}, nil, nil, nil)
	return h
}

func authCtx(pluginID string) context.Context {
	return CtxWithPluginID(context.Background(), pluginID)
}

func TestFetch_RejectsUnauthenticated(t *testing.T) {
	h := newFetchTestServer(t, []string{"api.open-meteo.com"})
	_, err := h.Fetch(context.Background(), &pluginv1.FetchRequest{Url: "https://api.open-meteo.com/x"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}

func TestFetch_RejectsNonHTTPScheme(t *testing.T) {
	h := newFetchTestServer(t, []string{"api.open-meteo.com"})
	for _, u := range []string{"file:///etc/passwd", "gopher://x/1", "ftp://x/y"} {
		_, err := h.Fetch(authCtx("plg-weather"), &pluginv1.FetchRequest{Url: u})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("scheme %q: want InvalidArgument, got %v", u, err)
		}
	}
}

func TestFetch_RejectsOffAllowList(t *testing.T) {
	h := newFetchTestServer(t, []string{"api.open-meteo.com"})
	_, err := h.Fetch(authCtx("plg-weather"), &pluginv1.FetchRequest{Url: "https://169.254.169.254/latest/meta-data/"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied for off-list host, got %v", err)
	}
}

func TestFetch_RejectsNonGetMethod(t *testing.T) {
	h := newFetchTestServer(t, []string{"api.open-meteo.com"})
	_, err := h.Fetch(authCtx("plg-weather"), &pluginv1.FetchRequest{
		Url:    "https://api.open-meteo.com/x",
		Method: "POST",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument for POST, got %v", err)
	}
}

func TestFetch_HappyPath_AllowedHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Host") != "" && r.Host == "spoofed" {
			t.Errorf("plugin managed to override Host header")
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("hello-body"))
	}))
	defer srv.Close()

	// Allow the test server's host and point the fetch client at a plain client
	// (the SSRF-safe one would reject the loopback test server).
	host := strings.TrimPrefix(srv.URL, "http://")
	host = strings.Split(host, ":")[0]

	h := newFetchTestServer(t, []string{host})
	prev := newFetchClient
	newFetchClient = func() *http.Client { return srv.Client() }
	defer func() { newFetchClient = prev }()

	resp, err := h.Fetch(authCtx("plg-weather"), &pluginv1.FetchRequest{
		Url:     srv.URL,
		Headers: map[string]string{"Host": "spoofed", "X-Test": "1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != "hello-body" {
		t.Fatalf("body = %q, want hello-body", resp.Body)
	}
	if resp.Headers["Content-Type"] != "image/jpeg" {
		t.Fatalf("content-type header not relayed: %v", resp.Headers)
	}
}

func TestFetch_RedirectToOffListHostBlocked(t *testing.T) {
	// A 302 to a host NOT in external_fetch must be refused: the SSRF dialer
	// re-checks the IP on each hop but not the HOST, so without a redirect
	// guard an allow-listed open-redirect (or a host the plugin author
	// controls) is an arbitrary-egress / exfiltration channel.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "https://evil.attacker.tld/exfil", http.StatusFound)
	}))
	defer srv.Close()
	host := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]

	h := newFetchTestServer(t, []string{host}) // only the redirecting host is allowed
	prev := newFetchClient
	newFetchClient = func() *http.Client { return srv.Client() }
	defer func() { newFetchClient = prev }()

	_, err := h.Fetch(authCtx("plg-weather"), &pluginv1.FetchRequest{Url: srv.URL})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied on off-list redirect, got %v", err)
	}
}

func TestFetch_RedirectToAllowedHostFollowed(t *testing.T) {
	// A redirect that stays on an allow-listed host is legitimate and must be
	// followed (the guard must not break same-host redirects).
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte("landed"))
			return
		}
		http.Redirect(w, r, srv.URL+"/final", http.StatusFound)
	}))
	defer srv.Close()
	host := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]

	h := newFetchTestServer(t, []string{host})
	prev := newFetchClient
	newFetchClient = func() *http.Client { return srv.Client() }
	defer func() { newFetchClient = prev }()

	resp, err := h.Fetch(authCtx("plg-weather"), &pluginv1.FetchRequest{Url: srv.URL})
	if err != nil {
		t.Fatalf("same-host redirect should be followed, got %v", err)
	}
	if string(resp.Body) != "landed" {
		t.Fatalf("body = %q, want landed", resp.Body)
	}
}

func TestFetch_PluginNotFound(t *testing.T) {
	h := NewHostServer(noopMediaQuerier{}, &fakePluginStore{err: context.DeadlineExceeded}, nil, nil, nil)
	_, err := h.Fetch(authCtx("plg-weather"), &pluginv1.FetchRequest{Url: "https://api.open-meteo.com/x"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestFetch_BodyCapTruncates(t *testing.T) {
	big := strings.Repeat("A", maxFetchBytes+100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, big)
	}))
	defer srv.Close()
	host := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]

	h := newFetchTestServer(t, []string{host})
	prev := newFetchClient
	newFetchClient = func() *http.Client { return srv.Client() }
	defer func() { newFetchClient = prev }()

	resp, err := h.Fetch(authCtx("plg-weather"), &pluginv1.FetchRequest{Url: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Truncated {
		t.Fatalf("expected truncated=true when body exceeds cap")
	}
	if len(resp.Body) != maxFetchBytes {
		t.Fatalf("body len = %d, want cap %d", len(resp.Body), maxFetchBytes)
	}
}
