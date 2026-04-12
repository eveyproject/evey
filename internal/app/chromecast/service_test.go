package chromecast

import (
	"context"
	"errors"
	"testing"

	domaincast "github.com/gravity-zero/evey/internal/domain/chromecast"
	castfakes "github.com/gravity-zero/evey/internal/domain/chromecast/fakes"
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
