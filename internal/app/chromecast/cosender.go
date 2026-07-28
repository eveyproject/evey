package chromecast

import (
	"log/slog"
	"sync"
	"time"

	domaincast "github.com/evey-project/evey/internal/domain/chromecast"
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
type CoSender struct {
	factory  domaincast.SessionFactory
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

// run owns the whole lifecycle of one device connection: connect, keepalive,
// release. It never touches media.
func (c *CoSender) run(addr string, stop <-chan struct{}) {
	defer c.wg.Done()
	defer func() {
		// Whatever the exit path, the slot must free so a later beacon can
		// re-attach (next episode, TV back on the network...).
		c.mu.Lock()
		if ch, ok := c.active[addr]; ok {
			// Only remove OUR registration: a Detach+Attach race may already
			// have installed a fresh loop under the same address.
			if ch == stop {
				delete(c.active, addr)
			}
		}
		c.mu.Unlock()
	}()

	sess, err := c.factory.Connect(domaincast.Device{
		UUID: "cosender-" + addr,
		Name: "Evey co-sender (" + addr + ")",
		Addr: addr,
		Port: castPort,
	})
	if err != nil {
		c.mu.Lock()
		c.lastFail[addr] = time.Now()
		c.mu.Unlock()
		slog.Info("cast co-sender: connect failed", slog.String("addr", addr), slog.Any("err", err))
		return
	}
	defer func() {
		if err := sess.Close(); err != nil {
			slog.Warn("cast co-sender: close failed", slog.String("addr", addr), slog.Any("err", err))
		}
	}()
	slog.Info("cast co-sender: attached", slog.String("addr", addr))

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	fails := 0
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
			if st.AppID != "" && st.AppID != c.appID {
				// The user moved the TV to another app: holding a sender seat
				// on someone else's session serves nothing and would keep the
				// TV awake for no reason.
				slog.Info("cast co-sender: another app took the TV, releasing",
					slog.String("addr", addr), slog.String("app_id", st.AppID))
				return
			}
		}
	}
}
