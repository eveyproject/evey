package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type TMDBArtworkDownloader struct {
	httpClient *http.Client
}

func NewTMDBArtworkDownloader(httpClient *http.Client) *TMDBArtworkDownloader {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &TMDBArtworkDownloader{httpClient: httpClient}
}

func (d *TMDBArtworkDownloader) Download(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return fmt.Errorf("unexpected content-type: %s", ct)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
