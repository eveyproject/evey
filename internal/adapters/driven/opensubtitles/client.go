package opensubtitles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/subtitle"
	"github.com/eveyproject/evey/internal/pkg/ssrf"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

const (
	baseURL        = "https://api.opensubtitles.com/api/v1"
	userAgent      = "evey-media-server v0.1.0"
	requestTimeout = 15 * time.Second
)

type Client struct {
	apiKey     string
	username   string
	password   string
	httpClient *http.Client

	mu    sync.Mutex
	token string
}

func New(apiKey, username, password string, httpClient ...*http.Client) *Client {
	var c *http.Client
	if len(httpClient) > 0 && httpClient[0] != nil {
		c = httpClient[0]
	} else {
		c = ssrf.SafeHTTPClient(requestTimeout)
	}
	return &Client{
		apiKey:     apiKey,
		username:   username,
		password:   password,
		httpClient: c,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

func (c *Client) login(ctx context.Context) error {
	body, err := json.Marshal(loginRequest{Username: c.username, Password: c.password})
	if err != nil {
		return fmt.Errorf("marshal login request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req, false)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("opensubtitles login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("opensubtitles login: status %d", resp.StatusCode)
	}
	var lr loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return fmt.Errorf("opensubtitles login decode: %w", err)
	}
	c.mu.Lock()
	c.token = lr.Token
	c.mu.Unlock()
	return nil
}

func (c *Client) ensureAuth(ctx context.Context) error {
	c.mu.Lock()
	hasToken := c.token != ""
	c.mu.Unlock()
	if hasToken {
		return nil
	}
	return c.login(ctx)
}

func (c *Client) setHeaders(req *http.Request, withAuth bool) {
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("User-Agent", userAgent)
	if withAuth {
		c.mu.Lock()
		tok := c.token
		c.mu.Unlock()
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
}

type searchResponse struct {
	Data []struct {
		Attributes struct {
			Language string  `json:"language"`
			Rating   float64 `json:"ratings"`
			Files    []struct {
				FileID   int    `json:"file_id"`
				FileName string `json:"file_name"`
			} `json:"files"`
			MovieHashMatch bool `json:"moviehash_match"`
		} `json:"attributes"`
	} `json:"data"`
}

func (c *Client) Search(ctx context.Context, hash, title, language string) ([]subtitle.SearchResult, error) {
	ctx, span := tracing.StartSpan(ctx, "opensubtitles.Search")
	defer span.End()

	params := url.Values{}
	if hash != "" {
		params.Set("moviehash", hash)
	}
	if title != "" {
		params.Set("query", title)
	}
	if language != "" {
		params.Set("languages", language)
	}

	reqURL := baseURL + "/subtitles?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, false)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opensubtitles search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("opensubtitles: rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opensubtitles search: status %d", resp.StatusCode)
	}

	// Bound the body like every other outbound call in this tree (tmdb 10MB,
	// tidb 64KB, the download below 10MB). A search response is a list of
	// subtitle files - a few hundred KB at worst - but it comes from a third
	// party, and the target here is a weak NAS: an unbounded decode is one
	// oversized reply away from eating the box's memory.
	var sr searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&sr); err != nil {
		return nil, fmt.Errorf("opensubtitles search decode: %w", err)
	}

	var results []subtitle.SearchResult
	for _, d := range sr.Data {
		for _, f := range d.Attributes.Files {
			format := "srt"
			ext := filepath.Ext(f.FileName)
			if ext != "" {
				format = ext[1:]
			}
			results = append(results, subtitle.SearchResult{
				FileID:   f.FileID,
				FileName: f.FileName,
				Language: d.Attributes.Language,
				Format:   format,
				Rating:   d.Attributes.Rating,
				Hash:     hash,
			})
		}
	}
	return results, nil
}

type downloadRequest struct {
	FileID int `json:"file_id"`
}

type downloadResponse struct {
	Link string `json:"link"`
}

func (c *Client) Download(ctx context.Context, fileID int, destPath string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "opensubtitles.Download")
	defer span.End()

	if err := c.ensureAuth(ctx); err != nil {
		return "", err
	}

	body, err := json.Marshal(downloadRequest{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("marshal download request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/download", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.setHeaders(req, true)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("opensubtitles download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", errors.New("opensubtitles: rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("opensubtitles download: status %d", resp.StatusCode)
	}

	var dr downloadResponse
	if err2 := json.NewDecoder(resp.Body).Decode(&dr); err2 != nil {
		return "", fmt.Errorf("opensubtitles download decode: %w", err2)
	}

	if err2 := os.MkdirAll(destPath, 0755); err2 != nil {
		return "", fmt.Errorf("create subtitle dir: %w", err2)
	}

	fileName := fmt.Sprintf("%d.srt", fileID)
	filePath := filepath.Join(destPath, fileName)

	fileReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dr.Link, nil)
	if err != nil {
		return "", err
	}
	fileResp, err := c.httpClient.Do(fileReq)
	if err != nil {
		return "", fmt.Errorf("download subtitle file: %w", err)
	}
	defer fileResp.Body.Close()

	if fileResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download subtitle file: status %d", fileResp.StatusCode)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("create subtitle file: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			f.Close()
		}
	}()

	// Cap at 10MB to prevent DoS from oversized subtitle responses
	if _, err := io.Copy(f, io.LimitReader(fileResp.Body, 10*1024*1024)); err != nil {
		if removeErr := os.Remove(filePath); removeErr != nil {
			slog.Warn("remove partial subtitle", slog.Any("err", removeErr))
		}
		return "", fmt.Errorf("write subtitle file: %w", err)
	}

	closed = true
	if err := f.Close(); err != nil {
		if removeErr := os.Remove(filePath); removeErr != nil {
			slog.Warn("remove partial subtitle", slog.Any("err", removeErr))
		}
		return "", fmt.Errorf("close subtitle file: %w", err)
	}

	slog.Info("subtitle downloaded", slog.Int("file_id", fileID), slog.String("path", filePath))
	return filePath, nil
}
