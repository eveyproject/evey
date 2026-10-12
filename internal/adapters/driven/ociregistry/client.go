// Package ociregistry reads what an OCI registry (Docker Hub, ghcr.io)
// publishes under a tag: the image's labels and the tag list. Read-only and
// anonymous, through the registry's own bearer-token challenge, so the same
// code serves every registry that speaks the distribution API.
package ociregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/app/updatecheck"
)

const (
	maxBody  = 4 << 20
	maxPages = 20
)

var manifestAccept = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

// Client is an updatecheck.Registry over HTTP.
type Client struct {
	http *http.Client
	// scheme is https; tests point it at a plain listener.
	scheme string
}

// New builds a client with a bounded timeout.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}, scheme: "https"}
}

var _ updatecheck.Registry = (*Client)(nil)

// ref splits "docker.io/ns/evey" into the registry host and the repository
// name, with Docker Hub's conventions (registry-1 host, library/ namespace).
func ref(image string) (host, name string) {
	image = strings.TrimPrefix(strings.TrimPrefix(image, "https://"), "http://")
	first, rest, found := strings.Cut(image, "/")
	if !found || (!strings.ContainsAny(first, ".:") && first != "localhost") {
		host, name = "docker.io", image
	} else {
		host, name = first, rest
	}
	if host == "docker.io" || host == "index.docker.io" {
		host = "registry-1.docker.io"
		if !strings.Contains(name, "/") {
			name = "library/" + name
		}
	}
	return host, name
}

// Describe reads the labels of image:tag for this server's platform.
func (c *Client) Describe(ctx context.Context, image, tag string) (updatecheck.Remote, error) {
	host, name := ref(image)
	base := fmt.Sprintf("%s://%s/v2/%s", c.scheme, host, name)
	var m struct {
		MediaType string `json:"mediaType"`
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := c.getJSON(ctx, base+"/manifests/"+url.PathEscape(tag), manifestAccept, &m); err != nil {
		return updatecheck.Remote{}, err
	}
	// A multi-arch index points at one manifest per platform.
	if len(m.Manifests) > 0 {
		digest := ""
		for _, d := range m.Manifests {
			if d.Platform.OS == "linux" && d.Platform.Architecture == runtime.GOARCH {
				digest = d.Digest
				break
			}
		}
		if digest == "" {
			return updatecheck.Remote{}, fmt.Errorf("no linux/%s image under %s", runtime.GOARCH, tag)
		}
		m.Config.Digest = ""
		if err := c.getJSON(ctx, base+"/manifests/"+digest, manifestAccept, &m); err != nil {
			return updatecheck.Remote{}, err
		}
	}
	if m.Config.Digest == "" {
		return updatecheck.Remote{}, errors.New("manifest without a config")
	}
	var cfg struct {
		Created string `json:"created"`
		Config  struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := c.getJSON(ctx, base+"/blobs/"+m.Config.Digest, "", &cfg); err != nil {
		return updatecheck.Remote{}, err
	}
	labels := cfg.Config.Labels
	created := labels["org.opencontainers.image.created"]
	if created == "" {
		created = cfg.Created
	}
	return updatecheck.Remote{
		Revision: labels["org.opencontainers.image.revision"],
		Created:  created,
		Source:   labels["org.opencontainers.image.source"],
	}, nil
}

var linkNext = regexp.MustCompile(`<([^>]+)>;\s*rel="?next"?`)

// Tags lists every tag of image, following the registry's pagination.
func (c *Client) Tags(ctx context.Context, image string) ([]string, error) {
	host, name := ref(image)
	next := fmt.Sprintf("%s://%s/v2/%s/tags/list?n=1000", c.scheme, host, name)
	var out []string
	for range maxPages {
		var page struct {
			Tags []string `json:"tags"`
		}
		resp, err := c.get(ctx, next, "application/json")
		if err != nil {
			return nil, err
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&page)
		link := resp.Header.Get("Link")
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode tags: %w", err)
		}
		out = append(out, page.Tags...)
		m := linkNext.FindStringSubmatch(link)
		if m == nil {
			return out, nil
		}
		u, err := url.Parse(m[1])
		if err != nil {
			return nil, fmt.Errorf("registry: bad tags page link %q: %w", m[1], err)
		}
		prev, err := url.Parse(next)
		if err != nil {
			return nil, fmt.Errorf("registry: bad tags page url %q: %w", next, err)
		}
		next = prev.ResolveReference(u).String()
	}
	return out, nil
}

func (c *Client) getJSON(ctx context.Context, u, accept string, v any) error {
	resp, err := c.get(ctx, u, accept)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", u, err)
	}
	return nil
}

// get performs a GET, answering the registry's bearer challenge once.
func (c *Client) get(ctx context.Context, u, accept string) (*http.Response, error) {
	resp, err := c.do(ctx, u, accept, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		token, err := c.token(ctx, challenge)
		if err != nil {
			return nil, err
		}
		if resp, err = c.do(ctx, u, accept, token); err != nil {
			return nil, err
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, updatecheck.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("registry: %s answered %d", u, resp.StatusCode)
	}
	return resp, nil
}

func (c *Client) do(ctx context.Context, u, accept, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return c.http.Do(req) //nolint:gosec // registry URL built from the admin's image setting
}

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// token fetches an anonymous pull token from the realm the challenge names.
func (c *Client) token(ctx context.Context, challenge string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", fmt.Errorf("registry: unsupported auth challenge %q", challenge)
	}
	params := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(challenge, -1) {
		params[strings.ToLower(m[1])] = m[2]
	}
	realm, err := url.Parse(params["realm"])
	if err != nil || realm.Scheme == "" {
		return "", fmt.Errorf("registry: bad auth realm %q", params["realm"])
	}
	q := realm.Query()
	for _, k := range []string{"service", "scope"} {
		if params[k] != "" {
			q.Set(k, params[k])
		}
	}
	realm.RawQuery = q.Encode()
	resp, err := c.do(ctx, realm.String(), "application/json", "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry: token endpoint answered %d", resp.StatusCode)
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&tok); err != nil {
		return "", fmt.Errorf("registry: decode token: %w", err)
	}
	if tok.Token != "" {
		return tok.Token, nil
	}
	return tok.AccessToken, nil
}
