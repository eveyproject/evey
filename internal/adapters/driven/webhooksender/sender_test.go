package webhooksender

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/pkg/ssrf"
)

// newTestSender returns a sender whose client is a plain http.Client, bypassing
// the SSRF dialer so tests can hit httptest servers bound to 127.0.0.1.
func newTestSender(timeout time.Duration) *HTTPSender {
	return &HTTPSender{client: &http.Client{Timeout: timeout}}
}

func TestSend_Success(t *testing.T) {
	var gotBody []byte
	var gotHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := newTestSender(10 * time.Second)
	body := []byte(`{"event":"test"}`)
	headers := map[string]string{"X-Signature": "abc123"}

	status, err := sender.Send(context.Background(), srv.URL, headers, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("want status 200, got %d", status)
	}
	if string(gotBody) != string(body) {
		t.Fatalf("want body %q, got %q", body, gotBody)
	}
	if gotHeaders.Get("Content-Type") != "application/json" {
		t.Fatalf("want Content-Type application/json, got %q", gotHeaders.Get("Content-Type"))
	}
	if gotHeaders.Get("X-Signature") != "abc123" {
		t.Fatalf("want X-Signature abc123, got %q", gotHeaders.Get("X-Signature"))
	}
}

func TestSend_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sender := newTestSender(10 * time.Second)
	status, err := sender.Send(context.Background(), srv.URL, nil, []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("want status 500, got %d", status)
	}
}

func TestSend_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := newTestSender(50 * time.Millisecond)
	_, err := sender.Send(context.Background(), srv.URL, nil, []byte(`{}`))
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestSend_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := newTestSender(10 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := sender.Send(ctx, srv.URL, nil, []byte(`{}`))
	if err == nil {
		t.Fatal("expected context canceled error, got nil")
	}
}

func TestSend_InvalidURL(t *testing.T) {
	sender := newTestSender(10 * time.Second)
	_, err := sender.Send(context.Background(), "://bad-url", nil, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// TestSend_SSRFBlocksLoopback verifies the default sender (New) refuses
// to connect to a 127.0.0.1 httptest server, proving the safe dialer is wired.
func TestSend_SSRFBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := New()
	_, err := sender.Send(context.Background(), srv.URL, nil, []byte(`{}`))
	if err == nil {
		t.Fatal("expected SSRF block error, got nil")
	}
	if !errors.Is(err, ssrf.ErrBlockedPrivateIP) {
		t.Fatalf("expected ErrBlockedPrivateIP in chain, got %v", err)
	}
}
