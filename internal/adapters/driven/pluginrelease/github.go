// Package pluginrelease reads the releases of a plugin's repository.
package pluginrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
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
	// ErrNoRepository: GitHub knows no such repository (or it is private).
	ErrNoRepository = errors.New("no public GitHub repository at this link")

	hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// GitHub reads releases through the GitHub REST API, without a token. The
// anonymous quota is 60 requests an hour per address, every request counted,
// conditional ones included: an instance spends one per followed plugin per
// hour, and stops for the hour when GitHub says the quota is spent.
type GitHub struct {
	client  *http.Client
	apiBase string

	mu    sync.Mutex
	cache map[int64]cachedRelease
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
	return &GitHub{client: client, apiBase: defaultAPIBase, cache: map[int64]cachedRelease{}}
}

// WithAPIBase points the reader at another API root (tests).
func (g *GitHub) WithAPIBase(base string) *GitHub {
	g.apiBase = strings.TrimRight(base, "/")
	return g
}

type ghRepo struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
}

type ghAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Digest      string `json:"digest"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Draft   bool      `json:"draft"`
	HTMLURL string    `json:"html_url"`
	Assets  []ghAsset `json:"assets"`
}

func (g *GitHub) get(ctx context.Context, apiPath, etag string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase+apiPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if err := rateLimited(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

// rateLimited recognises GitHub's refusal for a spent quota: a 429, or a 403
// with no request left, and says until when.
func rateLimited(resp *http.Response) error {
	spent := resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0")
	if !spent {
		return nil
	}
	if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		return fmt.Errorf("%w until %s", plugin.ErrReleaseRateLimited, time.Unix(reset, 0).UTC().Format(time.RFC3339))
	}
	return plugin.ErrReleaseRateLimited
}

// Identify returns the repository's permanent id and its canonical link. A
// renamed or transferred repository answers with its new link and the same id.
func (g *GitHub) Identify(ctx context.Context, repo string) (int64, string, error) {
	owner, name, err := plugin.RepoOwnerAndName(repo)
	if err != nil {
		return 0, "", err
	}
	resp, err := g.get(ctx, "/repos/"+owner+"/"+name, "")
	if err != nil {
		return 0, "", fmt.Errorf("identify repository: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return 0, "", ErrNoRepository
	case resp.StatusCode != http.StatusOK:
		return 0, "", fmt.Errorf("identify repository: HTTP %d", resp.StatusCode)
	}
	var r ghRepo
	if decErr := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseJSON)).Decode(&r); decErr != nil {
		return 0, "", fmt.Errorf("decode repository: %w", decErr)
	}
	canonical, err := plugin.NormalizeRepoURL(r.HTMLURL)
	if err != nil || canonical == "" || r.ID <= 0 {
		return 0, "", fmt.Errorf("identify repository: GitHub answered no usable id or link")
	}
	return r.ID, canonical, nil
}

// Latest returns the latest release of the repository with that id: its
// version, the single zip it carries and that zip's sha256.
func (g *GitHub) Latest(ctx context.Context, repoID int64) (*plugin.Release, error) {
	if repoID <= 0 {
		return nil, errors.New("read latest release: the repository is not identified")
	}
	g.mu.Lock()
	cached, haveCached := g.cache[repoID]
	g.mu.Unlock()
	etag := ""
	if haveCached {
		etag = cached.etag
	}
	resp, err := g.get(ctx, "/repositories/"+strconv.FormatInt(repoID, 10)+"/releases/latest", etag)
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
	rel, err := g.release(ctx, gr)
	if err != nil {
		return nil, err
	}
	if et := resp.Header.Get("ETag"); et != "" {
		g.mu.Lock()
		g.cache[repoID] = cachedRelease{etag: et, release: *rel}
		g.mu.Unlock()
	}
	return rel, nil
}

func (g *GitHub) release(ctx context.Context, gr ghRelease) (*plugin.Release, error) {
	// The repository the release belongs to, as GitHub names it today: the
	// release's own page is https://github.com/<owner>/<repo>/releases/tag/...
	owner, name, ok := releaseRepo(gr.HTMLURL)
	if !ok {
		return nil, fmt.Errorf("the latest release names no repository GitHub serves it from")
	}
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
	// The download must stay inside the repository's own release files.
	if !isReleaseAsset(zip.DownloadURL, owner, name) {
		return nil, fmt.Errorf("release asset %q is not served from the release files of %s/%s", zip.Name, owner, name)
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
	if a, ok := byName[zip.Name+".sha256"]; ok && isReleaseAsset(a.DownloadURL, owner, name) {
		d, err := g.fetchDigest(ctx, a.DownloadURL)
		if err != nil {
			return nil, err
		}
		rel.SHA256 = d
	}
	return rel, nil
}

// releaseRepo reads <owner>/<repo> out of a release page link.
func releaseRepo(htmlURL string) (owner, name string, ok bool) {
	u, err := url.Parse(htmlURL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawPath != "" {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "releases" || parts[3] != "tag" {
		return "", "", false
	}
	return parts[0], parts[1], parts[0] != "" && parts[1] != "" && parts[0] != ".." && parts[1] != ".."
}

// isReleaseAsset accepts exactly https://github.com/<owner>/<repo>/releases/
// download/<tag>/<file>, owner and repo matched without regard to case as
// GitHub does, on the parsed and cleaned path: no dot segment, no encoded
// separator, no query, no credentials can make it name another place.
func isReleaseAsset(raw, owner, name string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	if path.Clean(u.Path) != u.Path {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	return len(parts) == 6 &&
		strings.EqualFold(parts[0], owner) && strings.EqualFold(parts[1], name) &&
		parts[2] == "releases" && parts[3] == "download" &&
		parts[4] != "" && parts[5] != "" && parts[4] != ".." && parts[5] != ".."
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
