package opensubtitles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(serverURL string) *Client {
	c := New("test-api-key", "user", "pass")
	// Override baseURL by replacing the httpClient transport to redirect requests
	// Since baseURL is a const, we use an internal package test to override state
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{base: serverURL},
	}
	return c
}

// rewriteTransport rewrites requests from the production baseURL to the test server
type rewriteTransport struct {
	base string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	testURL, _ := url.Parse(t.base)
	req.URL.Scheme = testURL.Scheme
	req.URL.Host = testURL.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestSearchSubtitles_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/subtitles", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("Api-Key"))
		assert.Equal(t, "Some Movie", r.URL.Query().Get("query"))
		assert.Equal(t, "en", r.URL.Query().Get("languages"))

		resp := searchResponse{
			Data: []struct {
				Attributes struct {
					Language string  `json:"language"`
					Rating   float64 `json:"ratings"`
					Files    []struct {
						FileID   int    `json:"file_id"`
						FileName string `json:"file_name"`
					} `json:"files"`
					MovieHashMatch bool `json:"moviehash_match"`
				} `json:"attributes"`
			}{
				{
					Attributes: struct {
						Language string  `json:"language"`
						Rating   float64 `json:"ratings"`
						Files    []struct {
							FileID   int    `json:"file_id"`
							FileName string `json:"file_name"`
						} `json:"files"`
						MovieHashMatch bool `json:"moviehash_match"`
					}{
						Language: "en",
						Rating:   8.5,
						Files: []struct {
							FileID   int    `json:"file_id"`
							FileName string `json:"file_name"`
						}{
							{FileID: 100, FileName: "Some.Movie.1999.srt"},
							{FileID: 101, FileName: "Some.Movie.1999.sub"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	results, err := client.Search(context.Background(), "", "Some Movie", "en")

	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, 100, results[0].FileID)
	assert.Equal(t, "Some.Movie.1999.srt", results[0].FileName)
	assert.Equal(t, "en", results[0].Language)
	assert.Equal(t, "srt", results[0].Format)
	assert.InEpsilon(t, 8.5, results[0].Rating, 1e-6)

	assert.Equal(t, 101, results[1].FileID)
	assert.Equal(t, "sub", results[1].Format)
}

func TestSearchSubtitles_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.Search(context.Background(), "", "test", "en")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}

func TestDownloadSubtitle_Success(t *testing.T) {
	subtitleContent := "1\n00:00:01,000 --> 00:00:04,000\nHello, world!\n"
	destDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/login":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(loginResponse{Token: "test-token"})

		case "/api/v1/download":
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
			var dr downloadRequest
			json.NewDecoder(r.Body).Decode(&dr)
			assert.Equal(t, 42, dr.FileID)

			// Return a link pointing back to the test server for file content
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(downloadResponse{
				Link: "http://" + r.Host + "/files/42.srt",
			})

		case "/files/42.srt":
			w.Write([]byte(subtitleContent))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	filePath, err := client.Download(context.Background(), 42, destDir)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(destDir, "42.srt"), filePath)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, subtitleContent, string(content))
}

func TestDownloadSubtitle_FileLinkNotFound(t *testing.T) {
	destDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/login":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(loginResponse{Token: "test-token"})

		case "/api/v1/download":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(downloadResponse{
				Link: "http://" + r.Host + "/files/42.srt",
			})

		case "/files/42.srt":
			// CDN error page - must not be written to disk as a subtitle.
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("<html><body>Not Found</body></html>"))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	filePath, err := client.Download(context.Background(), 42, destDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
	assert.Empty(t, filePath)

	entries, err := os.ReadDir(destDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no subtitle file should be written when the CDN returns a non-2xx status")
}
