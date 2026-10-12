package chromecast

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/dns"

	domaincast "github.com/eveyproject/evey/internal/domain/chromecast"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

const (
	cacheTTL         = 60 * time.Second
	discoveryTimeout = 5 * time.Second

	// eveyCastReceiverAppID is the published Evey Web Receiver (Cast Console,
	// served from https://cast.eveyproject.org). Unlike the Default Media
	// Receiver it probes the TV's real codec capabilities and decodes AC-3 in
	// wasm - same app ID as the web and mobile senders.
	eveyCastReceiverAppID = "D36A449B"
)

// Device is re-exported from the domain for backwards compatibility with handlers.
type Device = domaincast.Device

type Service struct {
	mu       sync.RWMutex
	devices  []Device
	lastScan time.Time

	discoverer domaincast.Discoverer
	factory    domaincast.SessionFactory
	prober     domaincast.SubnetProber

	// connMu guards conns and connLocks ONLY - it is never held across a
	// blocking session call (Connect/StopMedia/Close). Per-device serialization
	// of those calls goes through connLocks instead.
	connMu    sync.Mutex
	conns     map[string]domaincast.CastSession
	connLocks map[string]*sync.Mutex
}

// New builds a Service backed by real go-chromecast discovery/connections.
// baseURL is retained for API compatibility; currently unused by the service.
func New(_ string) *Service {
	return NewWithPorts(&mdnsDiscoverer{}, &appSessionFactory{}, &tcpSubnetProber{})
}

// NewWithPorts builds a Service with injected ports (used in tests).
// prober may be nil (no unicast fallback).
func NewWithPorts(discoverer domaincast.Discoverer, factory domaincast.SessionFactory, prober domaincast.SubnetProber) *Service {
	return &Service{
		discoverer: discoverer,
		factory:    factory,
		prober:     prober,
		conns:      make(map[string]domaincast.CastSession),
		connLocks:  make(map[string]*sync.Mutex),
	}
}

// connLock returns the per-device connect mutex, creating it on first use.
// Briefly takes connMu only to read/insert into the connLocks map.
func (s *Service) connLock(uuid string) *sync.Mutex {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	l, ok := s.connLocks[uuid]
	if !ok {
		l = &sync.Mutex{}
		s.connLocks[uuid] = l
	}
	return l
}

// Discover lists cast devices. clientIP (may be empty) is the requesting
// client's real IP: when multicast mDNS finds nothing - the normal state for
// a server on a Docker bridge network, whose multicast never reaches the LAN -
// and the client is on a private network, the client's own /24 is probed
// directly over unicast, which DOES route out of a bridge. The client's subnet
// is the right one to scan: their Chromecast lives on their network, not on
// whatever internal network the container sees.
func (s *Service) Discover(ctx context.Context, clientIP string) ([]Device, error) {
	ctx, span := tracing.StartSpan(ctx, "app.chromecast.Discover")
	defer span.End()

	s.mu.RLock()
	if time.Since(s.lastScan) < cacheTTL && len(s.devices) > 0 {
		d := make([]Device, len(s.devices))
		copy(d, s.devices)
		s.mu.RUnlock()
		return d, nil
	}
	s.mu.RUnlock()

	devices, err := s.discoverer.Discover(ctx)
	if err != nil {
		return nil, err
	}

	if len(devices) == 0 && s.prober != nil && clientIP != "" {
		probed, probeErr := s.prober.ProbeSubnet(ctx, clientIP)
		if probeErr != nil {
			slog.WarnContext(ctx, "chromecast: subnet probe failed", slog.Any("err", probeErr))
		} else {
			devices = probed
		}
	}

	s.mu.Lock()
	// An empty result never overwrites known devices: a later hint-less scan
	// (or an mDNS blip) must not evict entries a user is about to Play on.
	if len(devices) > 0 {
		s.devices = devices
	}
	s.lastScan = time.Now()
	s.mu.Unlock()

	return devices, nil
}

// dedupeDevices collapses the repeated mDNS answers one Chromecast gives (one
// per interface the query went out on) down to a single entry.
//
// The key is the UUID, but ONLY when there is one: an entry whose TXT record
// carries no id= yields an empty UUID, and keying on that made every such
// device collide on the same empty string - so a second, genuinely different
// device silently vanished from the picker instead of a duplicate being
// removed. Address+port is the fallback identity.
func dedupeDevices(in []Device) []Device {
	seen := make(map[string]struct{}, len(in))
	out := make([]Device, 0, len(in))
	for _, d := range in {
		key := d.UUID
		if key == "" {
			key = fmt.Sprintf("%s:%d", d.Addr, d.Port)
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, d)
	}
	return out
}

func (s *Service) findDevice(uuid string) (*Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.devices {
		if d.UUID == uuid {
			dCopy := d
			return &dCopy, nil
		}
	}
	return nil, fmt.Errorf("device %q not found", uuid)
}

func (s *Service) getOrConnect(device *Device) (domaincast.CastSession, error) {
	// Fast path: a live cached session is returned without taking the
	// per-device connect lock.
	s.connMu.Lock()
	cached, ok := s.conns[device.UUID]
	s.connMu.Unlock()
	if ok && cached.Alive() {
		return cached, nil
	}

	// Serialize connects PER DEVICE: a slow Connect to one device must not
	// stall operations on every other device (the previous code held one
	// global lock across the blocking Connect), while two callers for the
	// SAME device still can't open duplicate sessions.
	devLock := s.connLock(device.UUID)
	devLock.Lock()
	defer devLock.Unlock()

	// Re-check under the per-device lock; another goroutine may have connected
	// while we waited. A dead session is dropped and closed OUTSIDE connMu.
	s.connMu.Lock()
	cached, ok = s.conns[device.UUID]
	if ok {
		delete(s.conns, device.UUID)
	}
	s.connMu.Unlock()
	if ok {
		if cached.Alive() {
			// Re-store the still-live session and reuse it.
			s.connMu.Lock()
			s.conns[device.UUID] = cached
			s.connMu.Unlock()
			return cached, nil
		}
		if err := cached.Close(); err != nil {
			slog.Warn("chromecast session close failed", slog.Any("err", err))
		}
	}

	sess, err := s.factory.Connect(*device)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", device.Name, err)
	}
	s.connMu.Lock()
	s.conns[device.UUID] = sess
	s.connMu.Unlock()
	return sess, nil
}

func (s *Service) Play(ctx context.Context, uuid, mediaURL, title string, contentType string) error {
	_, span := tracing.StartSpan(ctx, "app.chromecast.Play")
	defer span.End()

	_ = title
	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}

	sess, err := s.getOrConnect(device)
	if err != nil {
		return err
	}

	// The custom receiver comes first; the Default Media Receiver stays as
	// fallback for devices that cannot load it (no internet on the TV, app
	// rejected the media...).
	appErr := sess.LoadApp(eveyCastReceiverAppID, mediaURL)
	if appErr == nil {
		slog.InfoContext(ctx, "chromecast: playing (evey receiver)", slog.String("device", device.Name), slog.String("url", mediaURL))
		return nil
	}
	slog.WarnContext(ctx, "chromecast: evey receiver failed, falling back to default receiver",
		slog.String("device", device.Name), slog.Any("err", appErr))

	if contentType == "" {
		contentType = "video/mp4"
	}

	if err := sess.Load(mediaURL, contentType); err != nil {
		return fmt.Errorf("load media: %w", err)
	}
	slog.InfoContext(ctx, "chromecast: playing (default receiver)", slog.String("device", device.Name), slog.String("url", mediaURL))
	return nil
}

func (s *Service) Pause(ctx context.Context, uuid string) error {
	_, span := tracing.StartSpan(ctx, "app.chromecast.Pause")
	defer span.End()

	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}
	sess, err := s.getOrConnect(device)
	if err != nil {
		return err
	}
	return sess.Pause()
}

func (s *Service) Unpause(ctx context.Context, uuid string) error {
	_, span := tracing.StartSpan(ctx, "app.chromecast.Unpause")
	defer span.End()

	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}
	sess, err := s.getOrConnect(device)
	if err != nil {
		return err
	}
	return sess.Unpause()
}

func (s *Service) Stop(ctx context.Context, uuid string) error {
	_, span := tracing.StartSpan(ctx, "app.chromecast.Stop")
	defer span.End()

	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}

	// Take the per-device lock so a Stop racing a getOrConnect for the same
	// device is serialized, and do the blocking StopMedia/Close OUTSIDE connMu
	// (snapshot + delete under the lock, I/O after) so it can't stall other
	// devices.
	devLock := s.connLock(device.UUID)
	devLock.Lock()
	defer devLock.Unlock()

	s.connMu.Lock()
	sess, ok := s.conns[device.UUID]
	if ok {
		delete(s.conns, device.UUID)
	}
	s.connMu.Unlock()
	if !ok {
		return nil
	}

	stopErr := sess.StopMedia()
	if err := sess.Close(); err != nil {
		slog.WarnContext(ctx, "chromecast session close failed", slog.Any("err", err))
	}
	if stopErr != nil {
		return fmt.Errorf("stop: %w", stopErr)
	}
	return nil
}

func (s *Service) Seek(ctx context.Context, uuid string, positionSec int) error {
	_, span := tracing.StartSpan(ctx, "app.chromecast.Seek")
	defer span.End()

	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}
	sess, err := s.getOrConnect(device)
	if err != nil {
		return err
	}
	return sess.SeekToTime(float32(positionSec))
}

func (s *Service) SetVolume(ctx context.Context, uuid string, level float32) error {
	_, span := tracing.StartSpan(ctx, "app.chromecast.SetVolume")
	defer span.End()

	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}
	sess, err := s.getOrConnect(device)
	if err != nil {
		return err
	}
	return sess.SetVolume(level)
}

func (s *Service) Status(ctx context.Context, uuid string) (map[string]any, error) {
	_, span := tracing.StartSpan(ctx, "app.chromecast.Status")
	defer span.End()

	device, err := s.findDevice(uuid)
	if err != nil {
		return nil, err
	}
	sess, err := s.getOrConnect(device)
	if err != nil {
		return nil, err
	}
	st, err := sess.Status()
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"device": device.Name,
	}
	if st.App != "" {
		result["app"] = st.App
	}
	if st.PlayerState != "" {
		result["playerState"] = st.PlayerState
		result["currentTime"] = st.CurrentTime
		result["duration"] = st.Duration
	}
	result["volume"] = st.Volume
	result["muted"] = st.Muted
	return result, nil
}

func (s *Service) Close() {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	for uuid, sess := range s.conns {
		if err := sess.Close(); err != nil {
			slog.Warn("chromecast session close failed", slog.Any("err", err))
		}
		delete(s.conns, uuid)
	}
	// Drop the per-device lock map too so it doesn't retain entries for every
	// UUID ever seen across the service lifetime.
	s.connLocks = make(map[string]*sync.Mutex)
}

// --- real adapters (go-chromecast) ---

type mdnsDiscoverer struct{}

func (mdnsDiscoverer) Discover(ctx context.Context) ([]domaincast.Device, error) {
	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	entryChan, err := dns.DiscoverCastDNSEntries(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mdns discover: %w", err)
	}

	var devices []domaincast.Device
	for entry := range entryChan {
		devices = append(devices, deviceFromEntry(entry))
	}
	devices = dedupeDevices(devices)
	slog.InfoContext(ctx, "chromecast: discovery complete", slog.Int("count", len(devices)))
	return devices, nil
}

// deviceFromEntry projects an mDNS cast entry onto the domain device. The
// model rides along (CastEntry.Device is the TXT md record) so the sender
// can negotiate the stream for that receiver family.
func deviceFromEntry(entry dns.CastEntry) domaincast.Device {
	return domaincast.Device{
		UUID:  entry.GetUUID(),
		Name:  entry.GetName(),
		Addr:  entry.GetAddr(),
		Port:  entry.GetPort(),
		Model: entry.Device,
	}
}

// tcpSubnetProber finds Chromecasts without multicast: every cast device
// listens on TCP 8009 (the CASTV2 channel go-chromecast connects to anyway),
// so a plain unicast connect scan of the client's /24 finds them from inside
// a Docker bridge network where mDNS is structurally deaf. Names are synthetic
// ("Chromecast (ip)") - the CASTV2 handshake needed for the real name is what
// Play opens later; the picker label does not justify 254 TLS handshakes.
type tcpSubnetProber struct {
	// dial is injectable for tests; nil means net.DialTimeout.
	dial func(network, addr string, timeout time.Duration) (net.Conn, error)
}

const (
	castPort         = 8009
	probeTimeout     = 400 * time.Millisecond
	probeConcurrency = 64
)

func (p *tcpSubnetProber) ProbeSubnet(ctx context.Context, clientIP string) ([]domaincast.Device, error) {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return nil, nil
	}
	v4 := ip.To4()
	// Only probe private IPv4 space: a public client has no scannable home LAN
	// from here, and this must never become a generic port scanner.
	if v4 == nil || !ip.IsPrivate() {
		return nil, nil
	}

	dial := p.dial
	if dial == nil {
		dial = net.DialTimeout
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		found   []domaincast.Device
		sem     = make(chan struct{}, probeConcurrency)
		network = v4.Mask(net.CIDRMask(24, 32))
	)
	for host := 1; host <= 254; host++ {
		if ctx.Err() != nil {
			break
		}
		addr := net.IPv4(network[0], network[1], network[2], byte(host)).String()
		sem <- struct{}{}
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			defer func() { <-sem }()
			conn, err := dial("tcp", fmt.Sprintf("%s:%d", addr, castPort), probeTimeout)
			if err != nil {
				return
			}
			_ = conn.Close()
			mu.Lock()
			found = append(found, domaincast.Device{
				// Stable identity across scans so findDevice/Play and session
				// caching keep working when the list refreshes.
				UUID: "tcp-" + addr,
				Name: "Chromecast (" + addr + ")",
				Addr: addr,
				Port: castPort,
			})
			mu.Unlock()
		}(addr)
	}
	wg.Wait()
	// A cancelled request gets the cancellation, not a silently truncated scan
	// that would be cached as "the whole subnet".
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Addr < found[j].Addr })
	slog.InfoContext(ctx, "chromecast: subnet probe complete", slog.Int("count", len(found)))
	return found, nil
}

type appSessionFactory struct{}

func (appSessionFactory) Connect(device domaincast.Device) (domaincast.CastSession, error) {
	app := application.NewApplication(
		application.WithCacheDisabled(true),
		application.WithDeviceNameOverride(device.Name),
	)
	if err := app.Start(device.Addr, device.Port); err != nil {
		return nil, err
	}
	return &appSession{app: app}, nil
}

type appSession struct {
	app *application.Application
}

// receiverLaunchWindow bounds how long LoadApp waits for a launch failure to
// surface: LAUNCH errors within ~5s (lib request timeout) and a LOAD_FAILED
// arrives seconds after the receiver page is up. Past that, playback started.
const receiverLaunchWindow = 12 * time.Second

func (s *appSession) Load(mediaURL, contentType string) error {
	// detach=true: without it the lib parks in MediaWait until playback ENDS,
	// pinning the HTTP handler that called Play for the whole film.
	return s.app.Load(mediaURL, 0, contentType, false, true, false)
}

func (s *appSession) LoadApp(appID, mediaURL string) error {
	done := make(chan error, 1)
	go func() { done <- s.app.LoadApp(appID, mediaURL) }()
	return awaitLaunchOutcome(done, time.After(receiverLaunchWindow))
}

// awaitLaunchOutcome adapts go-chromecast's blocking LoadApp contract into a
// launch result. The lib call only returns BEFORE playback ends when something
// went wrong: an error for a failed LAUNCH, nil when the receiver reported
// LOAD_FAILED (mapped to "media finished" internally). Silence through the
// whole window is therefore the success signal; the goroutine keeps draining
// the lib call in the background until the media actually stops.
func awaitLaunchOutcome(done <-chan error, window <-chan time.Time) error {
	select {
	case err := <-done:
		if err != nil {
			return err
		}
		return fmt.Errorf("receiver rejected the media (LOAD_FAILED)")
	case <-window:
		return nil
	}
}

func (s *appSession) Pause() error   { return s.app.Pause() }
func (s *appSession) Unpause() error { return s.app.Unpause() }
func (s *appSession) StopMedia() error {
	return s.app.StopMedia()
}
func (s *appSession) SeekToTime(positionSec float32) error { return s.app.SeekToTime(positionSec) }
func (s *appSession) SetVolume(level float32) error        { return s.app.SetVolume(level) }

func (s *appSession) Status() (domaincast.MediaStatus, error) {
	castApp, media, vol := s.app.Status()
	ms := domaincast.MediaStatus{}
	if castApp != nil {
		ms.App = castApp.DisplayName
	}
	if media != nil {
		ms.PlayerState = media.PlayerState
		ms.CurrentTime = media.CurrentTime
		ms.Duration = media.Media.Duration
	}
	if vol != nil {
		ms.Volume = vol.Level
		ms.Muted = vol.Muted
	}
	return ms, nil
}

func (s *appSession) Alive() bool {
	return s.app.Update() == nil
}

func (s *appSession) Close() error {
	s.app.Close(false)
	return nil
}
