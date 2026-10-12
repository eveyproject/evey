package webhooksender

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/eveyproject/evey/internal/domain/webhook"
	"github.com/eveyproject/evey/internal/pkg/ssrf"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

var _ webhook.Sender = (*HTTPSender)(nil)

// Discard is the sender the demo instance is wired with: the webhook pipeline
// (events, delivery log, retries) runs unchanged, the host makes no outbound
// call. It answers 200 so the delivery log shows a success, not a failure.
type Discard struct{}

var _ webhook.Sender = Discard{}

func (Discard) Send(context.Context, string, map[string]string, []byte) (int, error) {
	return http.StatusOK, nil
}

type HTTPSender struct {
	client *http.Client
}

func New() *HTTPSender {
	return &HTTPSender{
		client: ssrf.SafeHTTPClient(10 * time.Second),
	}
}

// NewWithClient injects a custom http.Client - intended for tests that
// need to hit loopback (httptest.Server) without the SSRF guard firing.
func NewWithClient(c *http.Client) *HTTPSender {
	if c == nil {
		c = ssrf.SafeHTTPClient(10 * time.Second)
	}
	return &HTTPSender{client: c}
}

func (s *HTTPSender) Send(ctx context.Context, url string, headers map[string]string, body []byte) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "webhooksender.Send")
	defer span.End()

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
