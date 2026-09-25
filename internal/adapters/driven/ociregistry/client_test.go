package ociregistry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/app/updatecheck"
)

// fakeRegistry answers like Docker Hub: 401 + challenge without a token, an
// index for the tag, one manifest per platform, the config blob.
func fakeRegistry(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("scope") != "repository:someone/evey:pull" {
			http.Error(w, "bad scope", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"token":"t0k"}`)
	})
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer t0k" {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="registry.test",scope="repository:someone/evey:pull"`, srv.URL))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/v2/someone/evey/manifests/", authed(func(w http.ResponseWriter, r *http.Request) {
		ref := strings.TrimPrefix(r.URL.Path, "/v2/someone/evey/manifests/")
		switch ref {
		case "beta-0.0.1-full":
			fmt.Fprintf(w, `{"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[
				{"digest":"sha256:other","platform":{"os":"linux","architecture":"s390x"}},
				{"digest":"sha256:mine","platform":{"os":"linux","architecture":"%s"}}]}`, runtime.GOARCH)
		case "sha256:mine":
			fmt.Fprint(w, `{"config":{"digest":"sha256:cfg"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	mux.HandleFunc("/v2/someone/evey/blobs/sha256:cfg", authed(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"created":"2026-09-26T09:00:00Z","config":{"Labels":{
			"org.opencontainers.image.revision":"e77dc0a31",
			"org.opencontainers.image.source":"https://github.com/evey-project/evey"}}}`)
	}))
	mux.HandleFunc("/v2/someone/evey/tags/list", authed(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("last") == "" {
			w.Header().Set("Link", `</v2/someone/evey/tags/list?n=1000&last=b>; rel="next"`)
			fmt.Fprint(w, `{"tags":["a","b"]}`)
			return
		}
		fmt.Fprint(w, `{"tags":["c"]}`)
	}))
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testClient(srv *httptest.Server) (*Client, string) {
	c := New()
	c.scheme = "http"
	return c, strings.TrimPrefix(srv.URL, "http://") + "/someone/evey"
}

func TestDescribe_WalksChallengeIndexManifestAndConfig(t *testing.T) {
	c, image := testClient(fakeRegistry(t))
	remote, err := c.Describe(context.Background(), image, "beta-0.0.1-full")
	require.NoError(t, err)
	assert.Equal(t, "e77dc0a31", remote.Revision)
	assert.Equal(t, "2026-09-26T09:00:00Z", remote.Created)
	assert.Equal(t, "https://github.com/evey-project/evey", remote.Source)

	_, err = c.Describe(context.Background(), image, "nope")
	assert.ErrorIs(t, err, updatecheck.ErrNotFound)
}

func TestTags_FollowsPagination(t *testing.T) {
	c, image := testClient(fakeRegistry(t))
	tags, err := c.Tags(context.Background(), image)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, tags)
}

func TestRef_DockerHubConventions(t *testing.T) {
	for _, tc := range []struct{ in, host, name string }{
		{"docker.io/someone/evey", "registry-1.docker.io", "someone/evey"},
		{"someone/evey", "registry-1.docker.io", "someone/evey"},
		{"nginx", "registry-1.docker.io", "library/nginx"},
		{"ghcr.io/evey-project/evey", "ghcr.io", "evey-project/evey"},
		{"localhost:5000/evey", "localhost:5000", "evey"},
	} {
		h, n := ref(tc.in)
		assert.Equal(t, tc.host, h, tc.in)
		assert.Equal(t, tc.name, n, tc.in)
	}
}
