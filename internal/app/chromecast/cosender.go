package chromecast

import (
	"context"
	"log/slog"
	"sync"
	"time"

	domaincast "github.com/evey-project/evey/internal/domain/chromecast"
)

const (
	// probeFallbackTimeout bounds one /24 sweep (254 dials at 400ms, 64 wide:
	// ~2s nominal; the margin covers a congested Wi-Fi).
	probeFallbackTimeout = 15 * time.Second
	// emptyAppRelease is how many consecutive "no application at all" status
	// reads are tolerated before the seat is released. At the default 30s
	// interval this is 2 minutes - enough for any receiver boot or teardown
	// blip, short enough not to pin an idle TV.
	emptyAppRelease = 4
)

// CoSender keeps this server attached as a SECOND Cast sender on TVs that are
// playing the Evey receiver.
//
// Why it exists (prod measurements, 2026-07-28): when the mobile sender's OEM
// freezes the phone (ColorOS froze it despite a foreground service AND a held
// partial wakelock), the phone stops answering the CASTV2 heartbeat, the TV
// platform declares "last sender disconnected" and tears the receiver down
// ~3min40 later - measured twice, 3min35 and 3min45, while playback itself was
// flawless to the last second. The Cast spec says an implicit sender loss must
// NOT stop playback; the boxes involved (MiTV, Bouygtel4K) violate that. A
// sender that never sleeps makes the condition unreachable: with the server
// attached, the box never sees zero senders, whatever happens to the phone.
//
// How it attaches: the receiver posts a cast_receiver_alive beacon to this
// server every 30s, FROM the TV - each beacon hands us the TV's address, no
// discovery needed, and re-arms the attach for free after a server restart.
// go-chromecast does the rest: Application.Update() sends a CONNECT on the
// running app's transport id (that registration is exactly what makes us a
// sender of the session) and its connection loop answers the device's PINGs.
//
// Scope, stated honestly: this only protects deployments where the server can
// reach the TV (same network - the self-hosted NAS case this project targets).
// A remote user's TV is out of reach; for them the mobile-side hardening is
// the only lever. Attach attempts to unreachable addresses fail fast, are
// cooled down, and never affect playback.
//
// The co-sender is strictly PASSIVE: it never issues a media command. The
// tests pin that property.
//
// UNVERIFIED ON DEVICE (2026-07-29). Everything above rests on one assumption
// that no measurement has confirmed yet: that the box counts ANY connected
// sender, so a passive CONNECT on the running app's transport id cancels its
// "last sender disconnected" teardown. If its heuristic instead keys on the
// sender that LAUNCHED the app, or on the media session owner, this code holds
// a seat the box does not count and the TV still dies at ~3min40. The check is
// cheap and does not need this comment removed to run: freeze the phone and
// watch whether cast_receiver_alive beacons keep arriving past 4 minutes.
// Until that is observed, treat this as a plausible fix, not a proven one.
type CoSender struct {
	factory  domaincast.SessionFactory
	prober   domaincast.SubnetProber
	appID    string
	interval time.Duration
	cooldown time.Duration
	maxFails int

	mu       sync.Mutex
	active   map[string]chan struct{} // addr -> stop channel of the keepalive loop
	lastFail map[string]time.Time
	closed   bool
	wg       sync.WaitGroup
}

// CoSenderOption tunes a CoSender (test cadences).
type CoSenderOption func(*CoSender)

// WithCoSenderInterval sets the keepalive cadence. The default follows the
// device PING period by a wide margin: the point of the tick is not the
// heartbeat (the connection loop answers PINGs on its own) but noticing that
// the app changed or the TV left.
func WithCoSenderInterval(d time.Duration) CoSenderOption {
	return func(c *CoSender) { c.interval = d }
}

// WithCoSenderFailureCooldown sets how long a failed address is left alone.
// The receiver's beacons re-trigger Attach every 30s for the whole film; on a
// hairpin-NAT deployment the address they hand us is the router's, and
// without a cooldown that is one doomed TLS dial per beacon.
func WithCoSenderFailureCooldown(d time.Duration) CoSenderOption {
	return func(c *CoSender) { c.cooldown = d }
}

// WithCoSenderProber arms the subnet fallback: when the beacon address does
// not answer on the cast port, its /24 is scanned instead. Measured in prod
// (2026-07-28 21:19): hairpin NAT hands us the ROUTER's address
// (192.168.1.254), useless as a dial target - but its /24 is exactly where
// the TV lives, and the picker's prober already knows how to walk it.
func WithCoSenderProber(p domaincast.SubnetProber) CoSenderOption {
	return func(c *CoSender) { c.prober = p }
}

// NewCoSender builds a co-sender for the Evey receiver app id.
func NewCoSender(factory domaincast.SessionFactory, opts ...CoSenderOption) *CoSender {
	c := &CoSender{
		factory:  factory,
		appID:    eveyCastReceiverAppID,
		interval: 30 * time.Second,
		cooldown: 2 * time.Minute,
		maxFails: 3,
		active:   make(map[string]chan struct{}),
		lastFail: make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Attach ensures a sender connection to addr exists. Non-blocking: the
// connect happens in a goroutine so the beacon that triggered it is never
// delayed. Idempotent while a connection is live; a cooled-down failure is
// skipped silently (the next beacon after the cooldown retries).
func (c *CoSender) Attach(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if _, running := c.active[addr]; running {
		return
	}
	if until, failed := c.lastFail[addr]; failed && time.Since(until) < c.cooldown {
		return
	}
	stop := make(chan struct{})
	c.active[addr] = stop
	c.wg.Add(1)
	go c.run(addr, stop)
}

// Detach closes the connection to addr, if any. Called on the receiver's
// shutdown/finished beacons - once the session is over there is nothing left
// to protect.
func (c *CoSender) Detach(addr string) {
	c.mu.Lock()
	stop, ok := c.active[addr]
	if ok {
		delete(c.active, addr)
	}
	c.mu.Unlock()
	if ok {
		close(stop)
	}
}

// Close releases every connection. Blocks until the loops exited so shutdown
// never leaks a goroutine mid-TLS-handshake.
func (c *CoSender) Close() {
	c.mu.Lock()
	c.closed = true
	for addr, stop := range c.active {
		delete(c.active, addr)
		close(stop)
	}
	c.mu.Unlock()
	c.wg.Wait()
}

// releaseSlot frees addr so a later beacon can re-attach (next episode, TV
// back on the network...). Only OUR registration is removed: a Detach+Attach
// race may already have installed a fresh loop under the same address.
func (c *CoSender) releaseSlot(addr string, stop <-chan struct{}) {
	c.mu.Lock()
	if ch, ok := c.active[addr]; ok && ch == stop {
		delete(c.active, addr)
	}
	c.mu.Unlock()
}

func (c *CoSender) noteFailure(addr string) {
	c.mu.Lock()
	c.lastFail[addr] = time.Now()
	c.mu.Unlock()
}

// run owns the lifecycle of the connection triggered by a beacon: connect,
// keepalive, release. When the beacon address itself is not a cast device
// (hairpin NAT hands us the router), it falls back to probing the /24.
func (c *CoSender) run(addr string, stop <-chan struct{}) {
	defer c.wg.Done()
	defer c.releaseSlot(addr, stop)

	sess, err := c.factory.Connect(domaincast.Device{
		UUID: "cosender-" + addr,
		Name: "Evey co-sender (" + addr + ")",
		Addr: addr,
		Port: castPort,
	})
	if err != nil {
		c.noteFailure(addr)
		slog.Info("cast co-sender: connect failed, probing subnet", slog.String("addr", addr), slog.Any("err", err))
		c.probeFallback(addr)
		return
	}
	c.keepAlive(addr, sess, stop)
}

// probeFallback scans the /24 around hint for cast devices and seats the
// co-sender on every one that is running OUR receiver. The app-id check is
// what keeps this honest: a neighbour's Chromecast on the same LAN answers
// the port scan too, and we must neither hold a seat on it nor ever touch
// its playback.
func (c *CoSender) probeFallback(hint string) {
	if c.prober == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeFallbackTimeout)
	defer cancel()
	devices, err := c.prober.ProbeSubnet(ctx, hint)
	if err != nil {
		slog.Info("cast co-sender: subnet probe failed", slog.String("hint", hint), slog.Any("err", err))
		return
	}
	for _, dev := range devices {
		if dev.Addr == hint {
			continue // just failed to connect to it
		}
		c.attachVerified(dev)
	}
}

// attachVerified connects to a probed device, keeps the seat only if the Evey
// receiver is what's running there, and hands the session to the keepalive
// loop under the device's REAL address.
func (c *CoSender) attachVerified(dev domaincast.Device) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if _, running := c.active[dev.Addr]; running {
		c.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	c.active[dev.Addr] = stop
	c.mu.Unlock()

	sess, err := c.factory.Connect(dev)
	if err != nil {
		c.releaseSlot(dev.Addr, stop)
		return
	}
	st, err := sess.Status()
	if err != nil || st.AppID != c.appID {
		// Not our receiver (or not readable): release immediately, without
		// ever having sent anything but the status reads.
		_ = sess.Close()
		c.releaseSlot(dev.Addr, stop)
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer c.releaseSlot(dev.Addr, stop)
		c.keepAlive(dev.Addr, sess, stop)
	}()
}

// keepAlive holds the sender seat until the session is over, the device is
// gone, or Detach/Close asks for release. It never touches media.
func (c *CoSender) keepAlive(addr string, sess domaincast.CastSession, stop <-chan struct{}) {
	defer func() {
		if err := sess.Close(); err != nil {
			slog.Warn("cast co-sender: close failed", slog.String("addr", addr), slog.Any("err", err))
		}
	}()
	slog.Info("cast co-sender: attached", slog.String("addr", addr))

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	fails := 0
	emptyStreak := 0
	for {
		select {
		case <-stop:
			slog.Info("cast co-sender: detached", slog.String("addr", addr))
			return
		case <-ticker.C:
			// Alive() runs a full receiver-status update, which also re-asserts
			// the CONNECT on the app transport - the sender registration this
			// whole type exists for.
			if !sess.Alive() {
				fails++
				if fails >= c.maxFails {
					slog.Info("cast co-sender: device unreachable, releasing", slog.String("addr", addr))
					return
				}
				continue
			}
			fails = 0
			st, err := sess.Status()
			if err != nil {
				continue
			}
			switch {
			case st.AppID == "":
				// No application reported at all (receiver page closed, not
				// even the backdrop up yet): tolerate a boot/teardown blip,
				// but never hold a seat on an idle TV forever.
				emptyStreak++
				if emptyStreak >= emptyAppRelease {
					slog.Info("cast co-sender: no app on device, releasing", slog.String("addr", addr))
					return
				}
			case st.AppID != c.appID:
				// The user moved the TV to another app: holding a sender seat
				// on someone else's session serves nothing and would keep the
				// TV awake for no reason.
				slog.Info("cast co-sender: another app took the TV, releasing",
					slog.String("addr", addr), slog.String("app_id", st.AppID))
				return
			default:
				emptyStreak = 0
			}
		}
	}
}
