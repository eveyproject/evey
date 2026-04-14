package webhooksender

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/evey-project/evey/internal/domain/webhook"
)

var _ webhook.Sender = (*HTTPSender)(nil)

type HTTPSender struct {
	client *http.Client
}

func New() *HTTPSender {
	return &HTTPSender{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *HTTPSender) Send(ctx context.Context, url string, headers map[string]string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	return resp.StatusCode, nil
}
