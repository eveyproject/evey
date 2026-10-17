package loadtest

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Against a server that speaks HTTP/2, the run's shared client puts every
// viewer on one connection; a viewer client of its own dials its own, as a
// real device does.
func TestViewerClient_OneConnectionPerViewer(t *testing.T) {
	var conns atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(w, r.Proto)
	}))
	srv.EnableHTTP2 = true
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.StartTLS()
	defer srv.Close()

	// The production transport with the test certificate trusted; setting
	// TLSClientConfig would turn HTTP/2 off, which production does not do.
	tr := &http.Transport{MaxIdleConns: 2048, MaxIdleConnsPerHost: 2048, IdleConnTimeout: 90 * time.Second,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: true} //nolint:gosec // test server's self-signed certificate
	r := New(&http.Client{Transport: tr}, nil, nil, slog.New(slog.DiscardHandler))

	get := func(ctx context.Context) string {
		status, _, body, _, err := r.fetch(ctx, srv.URL, "t", true, "")
		if err != nil || status != http.StatusOK {
			t.Errorf("fetch: %d %v", status, err)
		}
		return string(body)
	}
	if p := get(context.Background()); p != "HTTP/2.0" {
		t.Fatalf("server answered %q, the test needs HTTP/2", p)
	}

	const viewers = 6
	run := func(ownClient bool) int64 {
		conns.Store(0)
		var wg sync.WaitGroup
		for range viewers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx := context.Background()
				if ownClient {
					var release func()
					ctx, release = r.withViewerClient(ctx)
					defer release()
				}
				for range 3 {
					get(ctx)
				}
			}()
		}
		wg.Wait()
		return conns.Load()
	}
	if n := run(false); n != 0 {
		t.Fatalf("shared client opened %d new connections, the premise is one shared connection", n)
	}
	if n := run(true); n != viewers {
		t.Fatalf("%d viewers with their own client opened %d connections, want one each", viewers, n)
	}
}

// A request no viewer makes keeps the run's own client.
func TestViewerClient_OutsideAViewerIsTheRunsClient(t *testing.T) {
	r := New(&http.Client{}, nil, nil, slog.New(slog.DiscardHandler))
	if r.clientFor(context.Background()) != r.httpClient {
		t.Fatal("a request outside a viewer must use the run's client")
	}
	ctx, release := r.withViewerClient(context.Background())
	defer release()
	if c := r.clientFor(ctx); c == r.httpClient || c.Transport == nil || c.Timeout != r.httpClient.Timeout {
		t.Fatalf("viewer client = %+v", c)
	}
}
