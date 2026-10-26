package metadata

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/pkg/ssrf"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ArtworkHTTPDownloader struct {
	httpClient *http.Client
}

func NewArtworkHTTPDownloader(httpClient *http.Client) *ArtworkHTTPDownloader {
	if httpClient == nil {
		// NoProxy: the URLs come from external sources; behind a proxy the
		// safe dialer would never see the real destination.
		httpClient = ssrf.SafeHTTPClientNoProxy(60 * time.Second)
	}
	return &ArtworkHTTPDownloader{httpClient: httpClient}
}

func (d *ArtworkHTTPDownloader) Download(ctx context.Context, url, destPath string) error {
	ctx, span := tracing.StartSpan(ctx, "metadata.artwork.Download")
	defer span.End()

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

	// Cap at 50MB to prevent DoS from oversized artwork responses
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 50*1024*1024)); err != nil {
		f.Close()
		if removeErr := os.Remove(destPath); removeErr != nil {
			slog.Warn("remove partial artwork", slog.Any("err", removeErr))
		}
		return fmt.Errorf("write file: %w", err)
	}

	return f.Close()
}
