package pluginrelease

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

const sum = "fbb7b8693889fed75e55546258e76d4691ec736bde4372a3ed63a379a10968d9"

// rewrite sends every request to the test server, keeping the path: the
// asset links stay github.com links, as the reader requires.
type rewrite struct{ base string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	target, _ := http.NewRequestWithContext(req.Context(), req.Method, r.base+req.URL.Path, nil)
	target.Header = req.Header
	return http.DefaultTransport.RoundTrip(target)
}

func reader(t *testing.T, h http.HandlerFunc) *GitHub {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewGitHub(&http.Client{Transport: rewrite{base: srv.URL}})
}

const (
	asset   = "https://github.com/acme/thing/releases/download/v1.2.0/thing-1.2.0.zip"
	relPage = "https://github.com/acme/thing/releases/tag/v1.2.0"
)

func latestJSON(assets string) string {
	return `{"tag_name":"v1.2.0","html_url":"` + relPage + `","assets":[` + assets + `]}`
}

func TestIdentify_ReturnsThePermanentIDAndTheCanonicalLink(t *testing.T) {
	g := reader(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/Acme/Thing", r.URL.Path)
		_, _ = io.WriteString(w, `{"id":42,"html_url":"https://github.com/acme/thing"}`)
	})
	id, canonical, err := g.Identify(context.Background(), "https://github.com/Acme/Thing")
	require.NoError(t, err)
	assert.Equal(t, int64(42), id)
	assert.Equal(t, "https://github.com/acme/thing", canonical)
}

func TestIdentify_AnUnknownRepository(t *testing.T) {
	g := reader(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	_, _, err := g.Identify(context.Background(), "https://github.com/acme/thing")
	require.ErrorIs(t, err, ErrNoRepository)
}

func TestLatest_ReadsByIDVersionZipAndDigest(t *testing.T) {
	g := reader(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repositories/42/releases/latest", r.URL.Path)
		_, _ = io.WriteString(w, latestJSON(`
			{"name":"thing-1.2.0.zip","browser_download_url":"`+asset+`","digest":"sha256:`+sum+`"},
			{"name":"thing-1.2.0.zip.sig","browser_download_url":"`+asset+`.sig"}`))
	})
	rel, err := g.Latest(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", rel.Version)
	assert.Equal(t, asset, rel.ZipURL)
	assert.Equal(t, sum, rel.SHA256)
}

// A renamed repository keeps its id; its release page and assets carry the
// new name, possibly another case than the one typed: that is the release.
func TestLatest_ARenamedRepositoryStillReads(t *testing.T) {
	g := reader(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tag_name":"v1.2.0","html_url":"https://github.com/NewOwner/thing/releases/tag/v1.2.0","assets":[
			{"name":"thing-1.2.0.zip","browser_download_url":"https://github.com/newowner/thing/releases/download/v1.2.0/thing-1.2.0.zip","digest":"sha256:`+sum+`"}]}`)
	})
	rel, err := g.Latest(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", rel.Version)
}

func TestLatest_FallsBackOnTheSha256Asset(t *testing.T) {
	g := reader(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			_, _ = io.WriteString(w, strings.ToUpper(sum)+"  thing-1.2.0.zip\n")
			return
		}
		_, _ = io.WriteString(w, latestJSON(`
			{"name":"thing-1.2.0.zip","browser_download_url":"`+asset+`"},
			{"name":"thing-1.2.0.zip.sha256","browser_download_url":"`+asset+`.sha256"}`))
	})
	rel, err := g.Latest(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, sum, rel.SHA256)
}

func TestLatest_Refusals(t *testing.T) {
	zip := func(u string) string { return latestJSON(`{"name":"a.zip","browser_download_url":"` + u + `"}`) }
	cases := map[string]struct {
		status int
		body   string
		want   error
		msg    string
	}{
		"no release":       {status: 404, want: ErrNoRelease},
		"no zip":           {status: 200, body: latestJSON(`{"name":"notes.txt","browser_download_url":"x"}`), want: ErrNoZip},
		"two zips":         {status: 200, body: latestJSON(`{"name":"a.zip","browser_download_url":"` + asset + `"},{"name":"b.zip","browser_download_url":"` + asset + `"}`), want: ErrNoZip},
		"asset elsewhere":  {status: 200, body: zip("https://evil.example/a.zip"), msg: "is not served from"},
		"other repository": {status: 200, body: zip("https://github.com/mallory/thing/releases/download/v1/a.zip"), msg: "is not served from"},
		"dot segments":     {status: 200, body: zip("https://github.com/acme/thing/releases/download/../../../mallory/evil/releases/download/v1/a.zip"), msg: "is not served from"},
		"encoded slash":    {status: 200, body: zip("https://github.com/acme/thing/releases/download/v1%2F..%2F..%2Fx/a.zip"), msg: "is not served from"},
		"query":            {status: 200, body: zip(asset + "?x=1"), msg: "is not served from"},
		"credentials":      {status: 200, body: zip("https://u:p@github.com/acme/thing/releases/download/v1/a.zip"), msg: "is not served from"},
		"no release page":  {status: 200, body: `{"tag_name":"v1","assets":[{"name":"a.zip","browser_download_url":"` + asset + `"}]}`, msg: "names no repository"},
		"draft":            {status: 200, body: `{"tag_name":"v1","draft":true,"assets":[]}`, want: ErrNoRelease},
		"server error":     {status: 502, msg: "HTTP 502"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := reader(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			})
			_, err := g.Latest(context.Background(), 42)
			require.Error(t, err)
			if c.want != nil {
				require.ErrorIs(t, err, c.want)
			}
			if c.msg != "" {
				assert.Contains(t, err.Error(), c.msg)
			}
		})
	}
}

// GitHub's refusal for a spent anonymous quota is said as such, with the time
// it lifts, so the run stops instead of failing every plugin in turn.
func TestLatest_ASpentQuotaIsRecognised(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"403 with no request left": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1900000000")
			w.WriteHeader(http.StatusForbidden)
		},
		"429": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := reader(t, h).Latest(context.Background(), 42)
			require.ErrorIs(t, err, plugin.ErrReleaseRateLimited)
		})
	}
	// A 403 that is not about the quota stays an ordinary failure.
	_, err := reader(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "12")
		w.WriteHeader(http.StatusForbidden)
	}).Latest(context.Background(), 42)
	require.Error(t, err)
	assert.NotErrorIs(t, err, plugin.ErrReleaseRateLimited)
}

func TestLatest_AnUnchangedReleaseIsAConditionalRequest(t *testing.T) {
	var calls, conditional atomic.Int32
	g := reader(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("If-None-Match") == `"e1"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"e1"`)
		_, _ = io.WriteString(w, latestJSON(`{"name":"thing-1.2.0.zip","browser_download_url":"`+asset+`","digest":"sha256:`+sum+`"}`))
	})
	for range 2 {
		rel, err := g.Latest(context.Background(), 42)
		require.NoError(t, err)
		assert.Equal(t, "1.2.0", rel.Version)
	}
	assert.Equal(t, int32(2), calls.Load())
	assert.Equal(t, int32(1), conditional.Load(), "the second look is answered 304 and served from the cache")
}

func TestLatest_RefusesAnUnidentifiedRepository(t *testing.T) {
	_, err := NewGitHub(http.DefaultClient).Latest(context.Background(), 0)
	require.Error(t, err)
}

func TestIdentify_RefusesALinkThatIsNotAGitHubRepository(t *testing.T) {
	_, _, err := NewGitHub(http.DefaultClient).Identify(context.Background(), "https://example.com/acme/thing")
	require.Error(t, err)
}
