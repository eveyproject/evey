package chromecast

import (
	"context"
	"sync"
	"testing"
	"time"

	domaincast "github.com/evey-project/evey/internal/domain/chromecast"
)

// The co-sender exists because of a reproducible prod measurement
// (2026-07-28): an OEM-frozen phone stops answering the CASTV2 heartbeat and
// ~3min40 later the TV platform tears the receiver down as "last sender
// disconnected" - twice measured, 3min35 and 3min45. A second sender that
// never sleeps (this server) makes that condition unreachable. These tests
// pin the three properties that make it safe: it is PASSIVE (never a media
// command), it detaches the moment the session is not ours, and it survives
// being triggered every 30s by the receiver's alive beacons without piling
// up connections.

type coFakeSession struct {
	mu         sync.Mutex
	appID      string
	aliveErr   error
	aliveCalls int
	closed     bool
	mediaCalls []string // any entry here is a test failure: the co-sender must stay passive
}

func (f *coFakeSession) media(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mediaCalls = append(f.mediaCalls, name)
	return nil
}

func (f *coFakeSession) Load(string, string) error          { return f.media("Load") }
func (f *coFakeSession) LoadApp(string, string) error       { return f.media("LoadApp") }
func (f *coFakeSession) Pause() error                       { return f.media("Pause") }
func (f *coFakeSession) Unpause() error                     { return f.media("Unpause") }
func (f *coFakeSession) StopMedia() error                   { return f.media("StopMedia") }
func (f *coFakeSession) SeekToTime(float32) error           { return f.media("SeekToTime") }
func (f *coFakeSession) SetVolume(float32) error            { return f.media("SetVolume") }

func (f *coFakeSession) Status() (domaincast.MediaStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return domaincast.MediaStatus{AppID: f.appID}, nil
}

func (f *coFakeSession) Alive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aliveCalls += 1
	return f.aliveErr == nil
}

func (f *coFakeSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *coFakeSession) snapshot() (closed bool, alive int, media []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed, f.aliveCalls, append([]string(nil), f.mediaCalls...)
}

type coFakeFactory struct {
	mu       sync.Mutex
	sessions map[string]*coFakeSession
	connects map[string]int
	fail     map[string]bool
}

func newCoFakeFactory() *coFakeFactory {
	return &coFakeFactory{
		sessions: map[string]*coFakeSession{},
		connects: map[string]int{},
		fail:     map[string]bool{},
	}
}

func (f *coFakeFactory) Connect(device domaincast.Device) (domaincast.CastSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connects[device.Addr] += 1
	if f.fail[device.Addr] {
		return nil, context.DeadlineExceeded
	}
	sess, ok := f.sessions[device.Addr]
	if !ok {
		sess = &coFakeSession{appID: eveyCastReceiverAppID}
		f.sessions[device.Addr] = sess
	}
	return sess, nil
}

func (f *coFakeFactory) connectCount(addr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connects[addr]
}

// session reads the map under the factory lock: the CoSender goroutine writes
// it concurrently through Connect.
func (f *coFakeFactory) session(addr string) *coFakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[addr]
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never reached")
}

func newTestCoSender(f domaincast.SessionFactory) *CoSender {
	return NewCoSender(f,
		WithCoSenderInterval(10*time.Millisecond),
		WithCoSenderFailureCooldown(50*time.Millisecond),
	)
}

func TestCoSenderAttachIsIdempotentWhileConnected(t *testing.T) {
	f := newCoFakeFactory()
	cs := newTestCoSender(f)
	defer cs.Close()

	// The receiver's alive beacon re-triggers Attach every 30s for the whole
	// film: only ONE connection may result.
	for range 5 {
		cs.Attach("192.168.1.50")
	}
	waitFor(t, func() bool { return f.connectCount("192.168.1.50") == 1 })
	time.Sleep(50 * time.Millisecond)
	if got := f.connectCount("192.168.1.50"); got != 1 {
		t.Fatalf("expected 1 connect, got %d", got)
	}
}

func TestCoSenderStaysPassiveAndKeepsSessionAlive(t *testing.T) {
	f := newCoFakeFactory()
	cs := newTestCoSender(f)
	defer cs.Close()

	cs.Attach("192.168.1.50")
	waitFor(t, func() bool {
		sess := f.session("192.168.1.50")
		if sess == nil {
			return false
		}
		_, alive, _ := sess.snapshot()
		return alive >= 3 // the keepalive loop is ticking
	})
	_, _, media := f.session("192.168.1.50").snapshot()
	if len(media) != 0 {
		t.Fatalf("co-sender must NEVER issue media commands, got %v", media)
	}
}

func TestCoSenderDetachesWhenAnotherAppTakesTheTV(t *testing.T) {
	f := newCoFakeFactory()
	cs := newTestCoSender(f)
	defer cs.Close()

	cs.Attach("192.168.1.50")
	waitFor(t, func() bool { return f.connectCount("192.168.1.50") == 1 })

	// The user switches the TV to another app: holding a sender connection on
	// someone else's session has no purpose - release it.
	sess := f.session("192.168.1.50")
	sess.mu.Lock()
	sess.appID = "CC1AD845" // Default Media Receiver
	sess.mu.Unlock()

	waitFor(t, func() bool { closed, _, _ := sess.snapshot(); return closed })
}

func TestCoSenderDetachesAfterConsecutiveFailures(t *testing.T) {
	f := newCoFakeFactory()
	cs := newTestCoSender(f)
	defer cs.Close()

	cs.Attach("192.168.1.50")
	waitFor(t, func() bool { return f.connectCount("192.168.1.50") == 1 })

	sess := f.session("192.168.1.50")
	sess.mu.Lock()
	sess.aliveErr = context.DeadlineExceeded // TV unplugged / left the network
	sess.mu.Unlock()

	waitFor(t, func() bool { closed, _, _ := sess.snapshot(); return closed })
}

func TestCoSenderExplicitDetach(t *testing.T) {
	f := newCoFakeFactory()
	cs := newTestCoSender(f)
	defer cs.Close()

	cs.Attach("192.168.1.50")
	waitFor(t, func() bool { return f.connectCount("192.168.1.50") == 1 })

	cs.Detach("192.168.1.50")
	waitFor(t, func() bool { closed, _, _ := f.session("192.168.1.50").snapshot(); return closed })

	// A later beacon may legitimately re-attach (next episode chained on the
	// same TV): a fresh connection must be possible after a detach.
	cs.Attach("192.168.1.50")
	waitFor(t, func() bool { return f.connectCount("192.168.1.50") == 2 })
}

func TestCoSenderConnectFailureIsCooledDown(t *testing.T) {
	f := newCoFakeFactory()
	f.fail["192.168.1.99"] = true
	cs := newTestCoSender(f)
	defer cs.Close()

	// A hairpin-NAT deployment hands us the router's IP: the connect fails.
	// The 30s beacons keep calling Attach - without a cooldown that is one
	// doomed TLS dial per beacon for the whole film.
	cs.Attach("192.168.1.99")
	waitFor(t, func() bool { return f.connectCount("192.168.1.99") == 1 })
	cs.Attach("192.168.1.99")
	cs.Attach("192.168.1.99")
	time.Sleep(20 * time.Millisecond)
	if got := f.connectCount("192.168.1.99"); got != 1 {
		t.Fatalf("expected the failure to be cooled down (1 connect), got %d", got)
	}

	// After the cooldown a beacon may try again.
	time.Sleep(60 * time.Millisecond)
	cs.Attach("192.168.1.99")
	waitFor(t, func() bool { return f.connectCount("192.168.1.99") == 2 })
}

func TestCoSenderCloseReleasesEverything(t *testing.T) {
	f := newCoFakeFactory()
	cs := newTestCoSender(f)

	cs.Attach("192.168.1.50")
	cs.Attach("192.168.1.51")
	waitFor(t, func() bool {
		return f.connectCount("192.168.1.50") == 1 && f.connectCount("192.168.1.51") == 1
	})
	cs.Close()
	waitFor(t, func() bool {
		c1, _, _ := f.session("192.168.1.50").snapshot()
		c2, _, _ := f.session("192.168.1.51").snapshot()
		return c1 && c2
	})
}

// --- subnet-probe fallback (hairpin NAT) ---

// Measured in prod on 2026-07-28 21:19: once the proxy chain forwarded a real
// LAN address, it was the ROUTER's (192.168.1.254, hairpin NAT rewrites the
// TV's source on the way through the public domain). The beacon address is
// then useless as a dial target, but its /24 is exactly where the TV lives -
// and the tcpSubnetProber already knows how to enumerate cast devices there.
// The co-sender must fall back to it, and must only seat itself on devices
// actually running OUR receiver.

type coFakeProber struct {
	mu      sync.Mutex
	devices []domaincast.Device
	err     error
	calls   int
	hints   []string
}

func (p *coFakeProber) ProbeSubnet(_ context.Context, clientIP string) ([]domaincast.Device, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls += 1
	p.hints = append(p.hints, clientIP)
	return p.devices, p.err
}

func (p *coFakeProber) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestCoSenderProbesSubnetWhenBeaconAddrIsNotACastDevice(t *testing.T) {
	f := newCoFakeFactory()
	f.fail["192.168.1.254"] = true // the router: hairpin NAT source
	f.sessions["192.168.1.23"] = &coFakeSession{appID: eveyCastReceiverAppID} // the TV
	f.sessions["192.168.1.40"] = &coFakeSession{appID: "CC1AD845"}           // neighbour on another app
	prober := &coFakeProber{devices: []domaincast.Device{
		{UUID: "tcp-192.168.1.23", Addr: "192.168.1.23", Port: 8009},
		{UUID: "tcp-192.168.1.40", Addr: "192.168.1.40", Port: 8009},
	}}
	cs := NewCoSender(f,
		WithCoSenderInterval(10*time.Millisecond),
		WithCoSenderFailureCooldown(50*time.Millisecond),
		WithCoSenderProber(prober),
	)
	defer cs.Close()

	cs.Attach("192.168.1.254")

	// The TV running our receiver gets a keepalive loop...
	waitFor(t, func() bool {
		sess := f.session("192.168.1.23")
		if sess == nil {
			return false
		}
		_, alive, _ := sess.snapshot()
		return alive >= 2
	})
	// ...the neighbour running another app is released, untouched.
	waitFor(t, func() bool { closed, _, _ := f.session("192.168.1.40").snapshot(); return closed })
	_, _, media := f.session("192.168.1.40").snapshot()
	if len(media) != 0 {
		t.Fatalf("verify must be passive, got %v on the foreign device", media)
	}
	if prober.hints[0] != "192.168.1.254" {
		t.Fatalf("probe hint = %q, want the beacon address", prober.hints[0])
	}
}

func TestCoSenderProbeFailureLandsInCooldown(t *testing.T) {
	f := newCoFakeFactory()
	f.fail["192.168.1.254"] = true
	prober := &coFakeProber{err: context.DeadlineExceeded}
	cs := NewCoSender(f,
		WithCoSenderInterval(10*time.Millisecond),
		WithCoSenderFailureCooldown(time.Hour),
		WithCoSenderProber(prober),
	)
	defer cs.Close()

	cs.Attach("192.168.1.254")
	waitFor(t, func() bool { return prober.callCount() == 1 })
	// The next beacons must not re-probe within the cooldown: a probe is 254
	// dials, not something to repeat every 30s.
	cs.Attach("192.168.1.254")
	cs.Attach("192.168.1.254")
	time.Sleep(30 * time.Millisecond)
	if got := prober.callCount(); got != 1 {
		t.Fatalf("probe must be cooled down, got %d calls", got)
	}
}

func TestCoSenderDoesNotProbeWhenDirectConnectWorks(t *testing.T) {
	f := newCoFakeFactory()
	prober := &coFakeProber{}
	cs := NewCoSender(f,
		WithCoSenderInterval(10*time.Millisecond),
		WithCoSenderFailureCooldown(50*time.Millisecond),
		WithCoSenderProber(prober),
	)
	defer cs.Close()

	cs.Attach("192.168.1.23")
	waitFor(t, func() bool { return f.connectCount("192.168.1.23") == 1 })
	time.Sleep(30 * time.Millisecond)
	if got := prober.callCount(); got != 0 {
		t.Fatalf("no probe expected on a direct hit, got %d", got)
	}
}

func TestCoSenderReleasesWhenAppVanishes(t *testing.T) {
	f := newCoFakeFactory()
	cs := newTestCoSender(f)
	defer cs.Close()

	cs.Attach("192.168.1.23")
	waitFor(t, func() bool { return f.connectCount("192.168.1.23") == 1 })

	// Receiver page closed, no application reported at all (not even the
	// backdrop): an empty app id forever must not hold a seat forever.
	sess := f.session("192.168.1.23")
	sess.mu.Lock()
	sess.appID = ""
	sess.mu.Unlock()

	waitFor(t, func() bool { closed, _, _ := sess.snapshot(); return closed })
}
