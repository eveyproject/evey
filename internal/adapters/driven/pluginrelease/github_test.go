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
)

const sum = "fbb7b8693889fed75e55546258e76d4691ec736bde4372a3ed63a379a10968d9"

// rewrite sends every request to the test server, keeping the path: the
// asset links stay github.com links, as the reader requires.
type rewrite struct{ base string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	target, _ := http.NewRequestWithContext(req.Context(), req.Method, r.base+u.Path, nil)
	target.Header = req.Header
	return http.DefaultTransport.RoundTrip(target)
}

func reader(t *testing.T, h http.HandlerFunc) *GitHub {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewGitHub(&http.Client{Transport: rewrite{base: srv.URL}})
}

const asset = "https://github.com/acme/thing/releases/download/v1.2.0/thing-1.2.0.zip"

func TestLatest_ReadsVersionZipAndDigest(t *testing.T) {
	g := reader(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/acme/thing/releases/latest", r.URL.Path)
		_, _ = io.WriteString(w, `{"tag_name":"v1.2.0","assets":[
			{"name":"thing-1.2.0.zip","browser_download_url":"`+asset+`","digest":"sha256:`+sum+`"},
			{"name":"thing-1.2.0.zip.sig","browser_download_url":"`+asset+`.sig"}]}`)
	})
	rel, err := g.Latest(context.Background(), "https://github.com/acme/thing")
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", rel.Version)
	assert.Equal(t, asset, rel.ZipURL)
	assert.Equal(t, sum, rel.SHA256)
}

func TestLatest_FallsBackOnTheSha256Asset(t *testing.T) {
	g := reader(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			_, _ = io.WriteString(w, strings.ToUpper(sum)+"  thing-1.2.0.zip\n")
			return
		}
		_, _ = io.WriteString(w, `{"tag_name":"v1.2.0","assets":[
			{"name":"thing-1.2.0.zip","browser_download_url":"`+asset+`"},
			{"name":"thing-1.2.0.zip.sha256","browser_download_url":"`+asset+`.sha256"}]}`)
	})
	rel, err := g.Latest(context.Background(), "https://github.com/acme/thing")
	require.NoError(t, err)
	assert.Equal(t, sum, rel.SHA256)
}

func TestLatest_Refusals(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   error
		msg    string
	}{
		"no release":       {status: 404, want: ErrNoRelease},
		"no zip":           {status: 200, body: `{"tag_name":"v1","assets":[{"name":"notes.txt","browser_download_url":"x"}]}`, want: ErrNoZip},
		"two zips":         {status: 200, body: `{"tag_name":"v1","assets":[{"name":"a.zip","browser_download_url":"` + asset + `"},{"name":"b.zip","browser_download_url":"` + asset + `"}]}`, want: ErrNoZip},
		"asset elsewhere":  {status: 200, body: `{"tag_name":"v1","assets":[{"name":"a.zip","browser_download_url":"https://evil.example/a.zip"}]}`, msg: "is not served from"},
		"other repository": {status: 200, body: `{"tag_name":"v1","assets":[{"name":"a.zip","browser_download_url":"https://github.com/mallory/thing/releases/download/v1/a.zip"}]}`, msg: "is not served from"},
		"draft":            {status: 200, body: `{"tag_name":"v1","draft":true,"assets":[]}`, want: ErrNoRelease},
		"server error":     {status: 502, msg: "HTTP 502"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := reader(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			})
			_, err := g.Latest(context.Background(), "https://github.com/acme/thing")
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
		_, _ = io.WriteString(w, `{"tag_name":"v1.2.0","assets":[{"name":"thing-1.2.0.zip","browser_download_url":"`+asset+`","digest":"sha256:`+sum+`"}]}`)
	})
	for range 2 {
		rel, err := g.Latest(context.Background(), "https://github.com/acme/thing")
		require.NoError(t, err)
		assert.Equal(t, "1.2.0", rel.Version)
	}
	assert.Equal(t, int32(2), calls.Load())
	assert.Equal(t, int32(1), conditional.Load(), "the second look is answered 304 and served from the cache")
}

func TestLatest_RefusesALinkThatIsNotAGitHubRepository(t *testing.T) {
	g := NewGitHub(http.DefaultClient)
	_, err := g.Latest(context.Background(), "https://example.com/acme/thing")
	require.Error(t, err)
}
