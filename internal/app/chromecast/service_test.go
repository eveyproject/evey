package chromecast

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/dns"

	domaincast "github.com/evey-project/evey/internal/domain/chromecast"
	castfakes "github.com/evey-project/evey/internal/domain/chromecast/fakes"
)

func newTestSvc(devices ...domaincast.Device) (*Service, *castfakes.FakeDiscoverer, *castfakes.FakeSessionFactory) {
	disc := &castfakes.FakeDiscoverer{Devices: devices}
	fac := castfakes.NewFakeSessionFactory()
	return NewWithPorts(disc, fac, nil), disc, fac
}

func TestDiscover_Cached(t *testing.T) {
	dev := domaincast.Device{UUID: "u1", Name: "Living", Addr: "1.2.3.4", Port: 8009}
	svc, disc, _ := newTestSvc(dev)

	d1, err := svc.Discover(context.Background(), "")
	if err != nil || len(d1) != 1 || d1[0].UUID != "u1" {
		t.Fatalf("first discover: %v %+v", err, d1)
	}
	d2, err := svc.Discover(context.Background(), "")
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
	if _, err := svc.Discover(context.Background(), ""); err == nil {
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

func TestPlay_LaunchesEveyReceiver(t *testing.T) {
	dev := domaincast.Device{UUID: "u1", Name: "Living"}
	svc, _, fac := newTestSvc(dev)
	if _, err := svc.Discover(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Play(context.Background(), "u1", "http://x/y.mp4", "title", ""); err != nil {
		t.Fatal(err)
	}
	sess := fac.Sessions["u1"]
	if sess == nil {
		t.Fatal("expected session created")
	}
	if sess.AppLaunched != eveyCastReceiverAppID {
		t.Fatalf("expected evey receiver launched, got %q", sess.AppLaunched)
	}
	if sess.AppMediaURL != "http://x/y.mp4" {
		t.Fatalf("expected media url handed to receiver, got %q", sess.AppMediaURL)
	}
	if sess.Loaded != "" {
		t.Fatalf("default receiver must not be used when the evey receiver works, got %q", sess.Loaded)
	}
}

func TestPlay_FallsBackToDefaultReceiver(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background(), "")
	sess := castfakes.NewFakeSession()
	sess.LoadAppErr = errors.New("app unavailable")
	fac.Sessions["u1"] = sess

	if err := svc.Play(context.Background(), "u1", "http://x/y.mp4", "", ""); err != nil {
		t.Fatal(err)
	}
	if sess.Loaded != "http://x/y.mp4" {
		t.Fatalf("expected DMR fallback load, got %q", sess.Loaded)
	}
	if sess.ContentType != "video/mp4" {
		t.Fatalf("expected default contentType video/mp4, got %q", sess.ContentType)
	}
}

func TestPlay_FallbackKeepsCustomContentType(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background(), "")
	sess := castfakes.NewFakeSession()
	sess.LoadAppErr = errors.New("app unavailable")
	fac.Sessions["u1"] = sess

	if err := svc.Play(context.Background(), "u1", "http://x/a.mkv", "", "video/x-matroska"); err != nil {
		t.Fatal(err)
	}
	if sess.ContentType != "video/x-matroska" {
		t.Fatal("expected custom contentType kept")
	}
}

func TestPlay_ErrorWhenBothReceiversFail(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background(), "")
	sess := castfakes.NewFakeSession()
	sess.LoadAppErr = errors.New("app unavailable")
	sess.LoadErr = errors.New("load refused")
	fac.Sessions["u1"] = sess

	if err := svc.Play(context.Background(), "u1", "http://x/y.mp4", "", ""); err == nil {
		t.Fatal("expected error when both receivers fail")
	}
}

// awaitLaunchOutcome adapts go-chromecast's blocking LoadApp contract: the lib
// call returning EARLY is the failure signal (error = LAUNCH failed, nil =
// LOAD_FAILED mapped to "media finished"), silence through the window means
// the receiver is up and playing.
func TestAwaitLaunchOutcome_LaunchError(t *testing.T) {
	done := make(chan error, 1)
	done <- errors.New("launch refused")
	if err := awaitLaunchOutcome(done, nil); err == nil {
		t.Fatal("expected launch error to propagate")
	}
}

func TestAwaitLaunchOutcome_EarlyNilMeansLoadFailed(t *testing.T) {
	done := make(chan error, 1)
	done <- nil
	if err := awaitLaunchOutcome(done, nil); err == nil {
		t.Fatal("an early nil return is a LOAD_FAILED, not a success")
	}
}

func TestAwaitLaunchOutcome_SilenceThroughWindowIsSuccess(t *testing.T) {
	window := make(chan time.Time, 1)
	window <- time.Time{}
	if err := awaitLaunchOutcome(make(chan error), window); err != nil {
		t.Fatalf("expected success on window expiry, got %v", err)
	}
}

func TestPauseUnpauseSeekVolume(t *testing.T) {
	dev := domaincast.Device{UUID: "u1"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background(), "")

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
	_, _ = svc.Discover(context.Background(), "")
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
	_, _ = svc.Discover(context.Background(), "")
	if err := svc.Stop(context.Background(), "u1"); err != nil {
		t.Fatalf("expected noop, got %v", err)
	}
}

func TestStatus_AggregatesSessionSnapshot(t *testing.T) {
	dev := domaincast.Device{UUID: "u1", Name: "Living"}
	svc, _, fac := newTestSvc(dev)
	_, _ = svc.Discover(context.Background(), "")
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
	_, _ = svc.Discover(context.Background(), "")
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
	_, _ = svc.Discover(context.Background(), "")
	fac.Err = errors.New("connect refused")
	if err := svc.Play(context.Background(), "u1", "http://x", "", ""); err == nil {
		t.Fatal("expected connect error")
	}
}

func TestClose_ClosesAllSessions(t *testing.T) {
	dev1 := domaincast.Device{UUID: "u1"}
	dev2 := domaincast.Device{UUID: "u2"}
	svc, _, fac := newTestSvc(dev1, dev2)
	_, _ = svc.Discover(context.Background(), "")
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
	svc := NewWithPorts(&castfakes.FakeDiscoverer{}, fac, nil)
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
	svc := NewWithPorts(&castfakes.FakeDiscoverer{}, fac, nil)

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

// mDNS answers arrive once per interface, so the same Chromecast is seen
// several times. Deduping on the UUID alone had a second failure mode: an
// entry whose TXT record carries no id= yields an EMPTY uuid, and every
// further such device collided on that same empty key - silently dropping
// real, distinct devices from the picker.
func TestDedupeDevices_SameUUIDCollapsed(t *testing.T) {
	out := dedupeDevices([]Device{
		{UUID: "abc", Name: "Salon", Addr: "192.168.1.20", Port: 8009},
		{UUID: "abc", Name: "Salon", Addr: "192.168.1.20", Port: 8009},
	})
	if len(out) != 1 {
		t.Fatalf("want 1, got %d", len(out))
	}
}

func TestDedupeDevices_EmptyUUIDKeepsDistinctDevices(t *testing.T) {
	out := dedupeDevices([]Device{
		{UUID: "", Name: "Salon", Addr: "192.168.1.20", Port: 8009},
		{UUID: "", Name: "Bedroom", Addr: "192.168.1.21", Port: 8009},
		{UUID: "", Name: "Salon", Addr: "192.168.1.20", Port: 8009},
	})
	if len(out) != 2 {
		t.Fatalf("two addresses must survive an empty uuid, got %d: %+v", len(out), out)
	}
}

// --- unicast subnet-probe fallback ------------------------------------------
// mDNS multicast never leaves a Docker bridge network, so in the common
// container deployment Discover returned [] forever while the server could
// perfectly well drive the Chromecast over unicast (observed in prod
// 2026-07-24). The prober is that unicast path; these tests pin when it runs.

func TestDiscover_ProbeFallback_WhenMDNSEmpty(t *testing.T) {
	dev := domaincast.Device{UUID: "tcp-192.168.1.42", Name: "Chromecast (192.168.1.42)", Addr: "192.168.1.42", Port: 8009}
	disc := &castfakes.FakeDiscoverer{} // mDNS finds nothing
	prober := &castfakes.FakeSubnetProber{Devices: []domaincast.Device{dev}}
	svc := NewWithPorts(disc, castfakes.NewFakeSessionFactory(), prober)

	got, err := svc.Discover(context.Background(), "192.168.1.23")
	if err != nil || len(got) != 1 || got[0].UUID != "tcp-192.168.1.42" {
		t.Fatalf("probe fallback: %v %+v", err, got)
	}
	if prober.Calls != 1 || prober.LastIP != "192.168.1.23" {
		t.Fatalf("prober not fed the client IP: calls=%d ip=%q", prober.Calls, prober.LastIP)
	}
}

func TestDiscover_NoProbe_WhenMDNSFindsDevices(t *testing.T) {
	dev := domaincast.Device{UUID: "u1", Addr: "10.0.0.9", Port: 8009}
	disc := &castfakes.FakeDiscoverer{Devices: []domaincast.Device{dev}}
	prober := &castfakes.FakeSubnetProber{}
	svc := NewWithPorts(disc, castfakes.NewFakeSessionFactory(), prober)

	if _, err := svc.Discover(context.Background(), "192.168.1.23"); err != nil {
		t.Fatal(err)
	}
	if prober.Calls != 0 {
		t.Fatalf("probe must not run when mDNS already answered, got %d calls", prober.Calls)
	}
}

func TestDiscover_NoProbe_WithoutClientIP(t *testing.T) {
	disc := &castfakes.FakeDiscoverer{}
	prober := &castfakes.FakeSubnetProber{}
	svc := NewWithPorts(disc, castfakes.NewFakeSessionFactory(), prober)

	if _, err := svc.Discover(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if prober.Calls != 0 {
		t.Fatalf("no client hint means nothing to scan, got %d calls", prober.Calls)
	}
}

func TestDiscover_EmptyResultKeepsKnownDevices(t *testing.T) {
	// A user picks a device from the list, thinks, taps Play after the cache
	// TTL: the re-scan coming back empty must not have evicted the device the
	// Play call is about to look up.
	dev := domaincast.Device{UUID: "u1", Addr: "10.0.0.9", Port: 8009}
	disc := &castfakes.FakeDiscoverer{Devices: []domaincast.Device{dev}}
	svc := NewWithPorts(disc, castfakes.NewFakeSessionFactory(), nil)

	if _, err := svc.Discover(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	// Expire the cache and make the next scan come back empty.
	svc.mu.Lock()
	svc.lastScan = time.Time{}
	svc.mu.Unlock()
	disc.Devices = nil

	if _, err := svc.Discover(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.findDevice("u1"); err != nil {
		t.Fatalf("known device evicted by an empty re-scan: %v", err)
	}
}

func TestTCPSubnetProber_RefusesPublicAndInvalidIPs(t *testing.T) {
	dialed := 0
	p := &tcpSubnetProber{dial: func(_, _ string, _ time.Duration) (net.Conn, error) {
		dialed++
		return nil, errors.New("refused")
	}}
	for _, ip := range []string{"", "not-an-ip", "8.8.8.8", "2001:db8::1"} {
		devs, err := p.ProbeSubnet(context.Background(), ip)
		if err != nil || devs != nil {
			t.Fatalf("ip %q: expected nil,nil got %v,%v", ip, devs, err)
		}
	}
	if dialed != 0 {
		t.Fatalf("must never scan around a public or invalid IP, dialed %d times", dialed)
	}
}

func TestTCPSubnetProber_FindsOpenCastPorts(t *testing.T) {
	// The prober scans the client's whole /24 on the CASTV2 port and reports
	// each open port as a device with a stable, addr-derived identity.
	p := &tcpSubnetProber{dial: func(_, addr string, _ time.Duration) (net.Conn, error) {
		if addr == "192.168.1.42:8009" || addr == "192.168.1.7:8009" {
			c, s := net.Pipe()
			go func() { _ = s.Close() }()
			return c, nil
		}
		return nil, errors.New("refused")
	}}
	devs, err := p.ProbeSubnet(context.Background(), "192.168.1.23")
	if err != nil || len(devs) != 2 {
		t.Fatalf("expected 2 devices: %v %+v", err, devs)
	}
	if devs[0].UUID != "tcp-192.168.1.42" && devs[1].UUID != "tcp-192.168.1.42" {
		t.Fatalf("missing stable uuid: %+v", devs)
	}
	for _, d := range devs {
		if d.Port != 8009 || d.Name == "" {
			t.Fatalf("bad device: %+v", d)
		}
	}
}

// The mDNS model record (TXT md) is the one capability hint a sender has for
// a device the backend found: it rides the device so the web sender can read
// the shared cast profile table with it. A subnet-probed device has none.
func TestDeviceFromEntry_CarriesTheModel(t *testing.T) {
	entry := dns.CastEntry{AddrV4: net.IPv4(192, 168, 1, 20), Port: 8009, UUID: "u1", Device: "Chromecast Ultra", DeviceName: "Salon"}
	got := deviceFromEntry(entry)
	if got.Model != "Chromecast Ultra" {
		t.Fatalf("Model = %q, want the md record", got.Model)
	}
	if got.UUID != "u1" || got.Addr != "192.168.1.20" || got.Port != 8009 {
		t.Fatalf("identity not kept: %+v", got)
	}
	if deviceFromEntry(dns.CastEntry{}).Model != "" {
		t.Fatal("no record, no model")
	}
}
