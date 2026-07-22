package chromecast

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/dns"

	domaincast "github.com/evey-project/evey/internal/domain/chromecast"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	cacheTTL         = 60 * time.Second
	discoveryTimeout = 5 * time.Second
)

// Device is re-exported from the domain for backwards compatibility with handlers.
type Device = domaincast.Device

type Service struct {
	mu       sync.RWMutex
	devices  []Device
	lastScan time.Time

	discoverer domaincast.Discoverer
	factory    domaincast.SessionFactory

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
	return NewWithPorts(&mdnsDiscoverer{}, &appSessionFactory{})
}

// NewWithPorts builds a Service with injected ports (used in tests).
func NewWithPorts(discoverer domaincast.Discoverer, factory domaincast.SessionFactory) *Service {
	return &Service{
		discoverer: discoverer,
		factory:    factory,
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

func (s *Service) Discover(ctx context.Context) ([]Device, error) {
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

	s.mu.Lock()
	s.devices = devices
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

	if contentType == "" {
		contentType = "video/mp4"
	}

	if err := sess.Load(mediaURL, contentType); err != nil {
		return fmt.Errorf("load media: %w", err)
	}
	slog.Info("chromecast: playing", slog.String("device", device.Name), slog.String("url", mediaURL))
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
		slog.Warn("chromecast session close failed", slog.Any("err", err))
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
		devices = append(devices, domaincast.Device{
			UUID: entry.GetUUID(),
			Name: entry.GetName(),
			Addr: entry.GetAddr(),
			Port: entry.GetPort(),
		})
	}
	devices = dedupeDevices(devices)
	slog.Info("chromecast: discovery complete", slog.Int("count", len(devices)))
	return devices, nil
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

func (s *appSession) Load(mediaURL, contentType string) error {
	return s.app.Load(mediaURL, 0, contentType, false, false, false)
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
