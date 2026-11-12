// Package pluginrelease reads the latest release of a plugin's repository.
package pluginrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/ssrf"
)

const (
	defaultAPIBase = "https://api.github.com"
	maxReleaseJSON = 1 << 20
	maxDigestFile  = 4 << 10
	userAgent      = "evey-plugin-updater/1.0"
)

var (
	// ErrNoRelease: the repository publishes no release yet.
	ErrNoRelease = errors.New("the repository has no published release")
	// ErrNoZip: the latest release carries no zip, or several.
	ErrNoZip = errors.New("the latest release must carry exactly one .zip")

	hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// GitHub reads releases through the GitHub REST API, without a token: an
// instance checks each followed plugin once an hour, well under the anonymous
// quota, and a conditional request answered 304 does not count against it.
type GitHub struct {
	client  *http.Client
	apiBase string

	mu    sync.Mutex
	cache map[string]cachedRelease
}

type cachedRelease struct {
	etag    string
	release plugin.Release
}

// NewGitHub builds the reader. A nil client gets the SSRF-safe one.
func NewGitHub(client *http.Client) *GitHub {
	if client == nil {
		client = ssrf.SafeHTTPClient(30 * time.Second)
	}
	return &GitHub{client: client, apiBase: defaultAPIBase, cache: map[string]cachedRelease{}}
}

// WithAPIBase points the reader at another API root (tests).
func (g *GitHub) WithAPIBase(base string) *GitHub {
	g.apiBase = strings.TrimRight(base, "/")
	return g
}

type ghAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Digest      string `json:"digest"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Draft   bool      `json:"draft"`
	Assets  []ghAsset `json:"assets"`
}

// Latest returns the repository's latest release: its version, the single
// zip it carries and that zip's sha256.
func (g *GitHub) Latest(ctx context.Context, repo string) (*plugin.Release, error) {
	owner, name, err := plugin.RepoOwnerAndName(repo)
	if err != nil {
		return nil, err
	}
	key := owner + "/" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase+"/repos/"+key+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	g.mu.Lock()
	cached, haveCached := g.cache[key]
	g.mu.Unlock()
	if haveCached {
		req.Header.Set("If-None-Match", cached.etag)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("read latest release: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified && haveCached:
		r := cached.release
		return &r, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNoRelease
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("read latest release: HTTP %d", resp.StatusCode)
	}
	var gr ghRelease
	if decErr := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseJSON)).Decode(&gr); decErr != nil {
		return nil, fmt.Errorf("decode latest release: %w", decErr)
	}
	if gr.Draft || gr.TagName == "" {
		return nil, ErrNoRelease
	}
	rel, err := g.release(ctx, owner, name, gr)
	if err != nil {
		return nil, err
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		g.mu.Lock()
		g.cache[key] = cachedRelease{etag: etag, release: *rel}
		g.mu.Unlock()
	}
	return rel, nil
}

func (g *GitHub) release(ctx context.Context, owner, name string, gr ghRelease) (*plugin.Release, error) {
	var zip *ghAsset
	byName := map[string]ghAsset{}
	for i, a := range gr.Assets {
		byName[a.Name] = a
		if strings.HasSuffix(strings.ToLower(a.Name), ".zip") {
			if zip != nil {
				return nil, ErrNoZip
			}
			zip = &gr.Assets[i]
		}
	}
	if zip == nil {
		return nil, ErrNoZip
	}
	// The download must stay inside the repository's own release files: an
	// asset is always served from there, anything else is not this release.
	prefix := "https://github.com/" + owner + "/" + name + "/releases/download/"
	if !strings.HasPrefix(zip.DownloadURL, prefix) {
		return nil, fmt.Errorf("release asset %q is not served from %s", zip.Name, prefix)
	}
	rel := &plugin.Release{
		Version: strings.TrimPrefix(gr.TagName, "v"),
		Tag:     gr.TagName,
		ZipURL:  zip.DownloadURL,
	}
	if d, ok := strings.CutPrefix(zip.Digest, "sha256:"); ok && hexDigest.MatchString(d) {
		rel.SHA256 = d
		return rel, nil
	}
	if a, ok := byName[zip.Name+".sha256"]; ok && strings.HasPrefix(a.DownloadURL, prefix) {
		d, err := g.fetchDigest(ctx, a.DownloadURL)
		if err != nil {
			return nil, err
		}
		rel.SHA256 = d
	}
	return rel, nil
}

// fetchDigest reads a "<hex>  <file>" line (sha256sum's output).
func (g *GitHub) fetchDigest(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("read release digest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("read release digest: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDigestFile))
	if err != nil {
		return "", fmt.Errorf("read release digest: %w", err)
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 || !hexDigest.MatchString(strings.ToLower(fields[0])) {
		return "", errors.New("release digest file holds no sha256")
	}
	return strings.ToLower(fields[0]), nil
}

var _ plugin.ReleaseSource = (*GitHub)(nil)
