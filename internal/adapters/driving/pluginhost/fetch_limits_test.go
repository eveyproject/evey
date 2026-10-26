package pluginhost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/eveyproject/evey/gen/plugin/v1"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// blockingFetchServer holds every request until release is closed and reports
// each arrival on arrived.
func blockingFetchServer(t *testing.T) (h *HostServer, url string, arrived chan struct{}, release func()) {
	t.Helper()
	gate := make(chan struct{})
	arrived = make(chan struct{}, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		arrived <- struct{}{}
		<-gate
		_, _ = w.Write([]byte("ok"))
	}))
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(func() { release(); srv.Close() })
	host := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]
	prev := newFetchClient
	newFetchClient = func() *http.Client { return srv.Client() }
	t.Cleanup(func() { newFetchClient = prev })
	return newFetchTestServer(t, []string{host}), srv.URL, arrived, release
}

// TestFetch_PerPluginConcurrencyCap: a plugin gets maxFetchInFlightPerPlugin
// brokered fetches at once; the next one is refused, another plugin is not.
func TestFetch_PerPluginConcurrencyCap(t *testing.T) {
	h, url, arrived, release := blockingFetchServer(t)
	var wg sync.WaitGroup
	for range maxFetchInFlightPerPlugin {
		wg.Go(func() {
			_, err := h.Fetch(authCtx("plg-flood"), &pluginv1.FetchRequest{Url: url})
			if err != nil {
				t.Errorf("fetch within the cap: %v", err)
			}
		})
	}
	for range maxFetchInFlightPerPlugin {
		<-arrived
	}

	ctx, cancel := context.WithTimeout(authCtx("plg-flood"), 2*time.Second)
	defer cancel()
	_, err := h.Fetch(ctx, &pluginv1.FetchRequest{Url: url})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("fetch over the per-plugin cap: want ResourceExhausted, got %v", err)
	}
	other := make(chan error, 1)
	go func() {
		_, err := h.Fetch(authCtx("plg-other"), &pluginv1.FetchRequest{Url: url})
		other <- err
	}()
	<-arrived
	release()
	if err := <-other; err != nil {
		t.Fatalf("another plugin must keep its own slots: %v", err)
	}
	wg.Wait()

	if _, err := h.Fetch(authCtx("plg-flood"), &pluginv1.FetchRequest{Url: url}); err != nil {
		t.Fatalf("slots must be released after the fetches end: %v", err)
	}
}

// TestFetch_GlobalBytesBudget: every in-flight fetch reserves its worst-case
// body against a host-wide budget, so many plugins together cannot pin more
// than that much memory; a fetch that cannot reserve gives up with its ctx.
func TestFetch_GlobalBytesBudget(t *testing.T) {
	h, url, arrived, release := blockingFetchServer(t)
	prev := fetchBytesBudget
	fetchBytesBudget = semaphore.NewWeighted(2 * maxFetchBytes)
	t.Cleanup(func() { fetchBytesBudget = prev })

	var wg sync.WaitGroup
	for _, id := range []string{"plg-a", "plg-b"} {
		wg.Go(func() { _, _ = h.Fetch(authCtx(id), &pluginv1.FetchRequest{Url: url}) })
	}
	<-arrived
	<-arrived

	ctx, cancel := context.WithTimeout(authCtx("plg-c"), 100*time.Millisecond)
	defer cancel()
	_, err := h.Fetch(ctx, &pluginv1.FetchRequest{Url: url})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("fetch over the global budget: want ResourceExhausted, got %v", err)
	}
	release()
	wg.Wait()
	if !fetchBytesBudget.TryAcquire(2 * maxFetchBytes) {
		t.Fatal("the budget must be fully returned after the fetches end")
	}
}
