package chromecast

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domaincast "github.com/evey-project/evey/internal/domain/chromecast"
	castfakes "github.com/evey-project/evey/internal/domain/chromecast/fakes"
)

func newTestSvc(devices ...domaincast.Device) (*Service, *castfakes.FakeDiscoverer, *castfakes.FakeSessionFactory) {
	disc := &castfakes.FakeDiscoverer{Devices: devices}
	fac := castfakes.NewFakeSessionFactory()
	return NewWithPorts(disc, fac), disc, fac
}

func TestDiscover_Cached(t *testing.T) {
	dev := domaincast.Device{UUID: "u1", Name: "Living", Addr: "1.2.3.4", Port: 8009}
	svc, disc, _ := newTestSvc(dev)

	d1, err := svc.Discover(context.Background())
	if err != nil || len(d1) != 1 || d1[0].UUID != "u1" {
		t.Fatalf("first discover: %v %+v", err, d1)
	}
	d2, err := svc.Discover(context.Background())
	if err != nil || len(d2) != 1 {
		t.Fatalf("second discover: %v %+v", err, d2)
	}
	if disc.Calls != 1 {
		t.Fatalf("expected cache hit, got %d calls", disc.Calls)
	}
}

func TestDiscover_Error(t *testing.T) {
	svc, disc, _ := newTestSvc()
	disc.Err = errors.New("net down")
	if _, err := svc.Discover(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestPlay_DeviceNotFound(t *testing.T) {
	svc, _, _ := newTestSvc()
	err := svc.Play(context.Background(), "ghost", "http://x/y.mp4", "t", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPlay_Happy(t *testing.T) {
	dev := domaincast.Device{UUID: "u1", Name: "Living"}
	svc, _, fac := newTestSvc(dev)
	if _, err := svc.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Play(context.Background(), "u1", "http://x/y.mp4", "title", ""); err != nil {
		t.Fatal(err)
	}
	sess := fac.Sessions["u1"]
	if sess == nil {
		t.Fatal("expected session created")
	}
	if sess.Loaded != "http://x/y.mp4" {
		t.Fatalf("expected loaded url, got %q", sess.Loaded)
	}
	if sess.ContentType != "video/mp4" {
		t.Fatalf("expected default contentType video/mp4, got %q", sess.ContentType)
	}
}

func TestPlay_CustomContentType(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background())
	if err := svc.Play(context.Background(), "u1", "http://x/a.mkv", "", "video/x-matroska"); err != nil {
		t.Fatal(err)
	}
	if fac.Sessions["u1"].ContentType != "video/x-matroska" {
		t.Fatal("expected custom contentType kept")
	}
}

func TestPauseUnpauseSeekVolume(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background())

	if err := svc.Pause(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Unpause(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Seek(context.Background(), "u1", 42); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetVolume(context.Background(), "u1", 0.5); err != nil {
		t.Fatal(err)
	}
	s := fac.Sessions["u1"]
	if !s.Paused || !s.Unpaused || s.SeekedTo != 42 || s.Volume != 0.5 {
		t.Fatalf("unexpected session state: %+v", s)
	}
}

func TestStop_ClosesAndRemovesSession(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background())
	// open session
	_ = svc.Pause(context.Background(), "u1")

	if err := svc.Stop(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	s := fac.Sessions["u1"]
	if !s.Stopped || !s.Closed {
		t.Fatalf("expected stopped+closed, got %+v", s)
	}
	if _, ok := svc.conns["u1"]; ok {
		t.Fatal("expected session removed from conns")
	}
}

func TestStop_NoSessionIsNoop(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, _ := newTestSvc(dev)
	_, _ = svc.Discover(context.Background())
	if err := svc.Stop(context.Background(), "u1"); err != nil {
		t.Fatalf("expected noop, got %v", err)
	}
}

func TestStatus_AggregatesSessionSnapshot(t *testing.T) {
	dev := domaincast.Device{UUID: "u1", Name: "Living"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background())
	_ = svc.Pause(context.Background(), "u1") // ensures session exists
	fac.Sessions["u1"].StatusReturn = domaincast.MediaStatus{
		App: "YouTube", PlayerState: "PLAYING", CurrentTime: 12, Duration: 300, Volume: 0.8, Muted: false,
	}

	st, err := svc.Status(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if st["device"] != "Living" || st["app"] != "YouTube" || st["playerState"] != "PLAYING" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestGetOrConnect_ReconnectsWhenDead(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background())
	_ = svc.Pause(context.Background(), "u1")

	first := fac.Sessions["u1"]
	first.AliveFlag = false

	// Trigger reconnect: factory will return same stored session from its map.
	// Remove from factory map to ensure a fresh session is created.
	delete(fac.Sessions, "u1")

	if err := svc.Pause(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if !first.Closed {
		t.Fatal("expected dead session to be closed")
	}
	if fac.Sessions["u1"] == first {
		t.Fatal("expected new session on reconnect")
	}
}

func TestConnectError_PropagatesOnPlay(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background())
	fac.Err = errors.New("connect refused")
	if err := svc.Play(context.Background(), "u1", "http://x", "", ""); err == nil {
		t.Fatal("expected connect error")
	}
}

func TestClose_ClosesAllSessions(t *testing.T) {
	dev1 := domaincast.Device{UUID: "u1"}
	dev2 := domaincast.Device{UUID: "u2"}
	svc, _, fac := newTestSvc(dev1, dev2)
	_, _ = svc.Discover(context.Background())
	_ = svc.Pause(context.Background(), "u1")
	_ = svc.Pause(context.Background(), "u2")

	svc.Close()

	if !fac.Sessions["u1"].Closed || !fac.Sessions["u2"].Closed {
		t.Fatal("expected all sessions closed")
	}
	if len(svc.conns) != 0 {
		t.Fatal("expected conns cleared")
	}
}

// latchFactory blocks each Connect on a release channel and records how many
// connects are simultaneously in flight, so a test can prove getOrConnect
// serializes per device (singleflight) yet runs connects for DISTINCT devices
// in parallel (the old code held one global lock across the blocking Connect).
type latchFactory struct {
	entered chan string   // one send per Connect that reaches the factory
	release chan struct{} // closed to let every blocked Connect return
	mu      sync.Mutex
	calls   int
}

func newLatchFactory() *latchFactory {
	return &latchFactory{entered: make(chan string, 16), release: make(chan struct{})}
}

func (f *latchFactory) Connect(device domaincast.Device) (domaincast.CastSession, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	f.entered <- device.UUID
	<-f.release
	return castfakes.NewFakeSession(), nil
}

func (f *latchFactory) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestGetOrConnect_SingleflightPerDevice(t *testing.T) {
	fac := newLatchFactory()
	svc := NewWithPorts(&castfakes.FakeDiscoverer{}, fac)
	dev := &domaincast.Device{UUID: "u1"}

	const N = 5
	done := make(chan struct{}, N)
	for range N {
		go func() {
			_, _ = svc.getOrConnect(dev)
			done <- struct{}{}
		}()
	}

	// Exactly one goroutine should reach the factory; the rest block on the
	// per-device lock and never call Connect.
	<-fac.entered
	select {
	case uuid := <-fac.entered:
		t.Fatalf("a second concurrent same-device Connect ran (%s): per-device singleflight broken", uuid)
	case <-time.After(100 * time.Millisecond):
	}

	close(fac.release)
	for range N {
		<-done
	}
	if fac.callCount() != 1 {
		t.Fatalf("expected exactly 1 Connect for one device, got %d", fac.callCount())
	}
}

func TestGetOrConnect_DistinctDevicesRunInParallel(t *testing.T) {
	fac := newLatchFactory()
	svc := NewWithPorts(&castfakes.FakeDiscoverer{}, fac)

	go func() { _, _ = svc.getOrConnect(&domaincast.Device{UUID: "u1"}) }()
	go func() { _, _ = svc.getOrConnect(&domaincast.Device{UUID: "u2"}) }()

	// Both connects must be in flight at once: if getOrConnect held a global
	// lock across Connect, the second UUID would never arrive until the first
	// returned (it can't, it's blocked on release) and this would time out.
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case uuid := <-fac.entered:
			seen[uuid] = true
		case <-time.After(time.Second):
			t.Fatalf("only %d/2 distinct-device connects ran in parallel: %v", len(seen), seen)
		}
	}
	close(fac.release)
}
