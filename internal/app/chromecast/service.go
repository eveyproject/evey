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

	connMu sync.Mutex
	conns  map[string]domaincast.CastSession
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
	}
}

func (s *Service) Discover(ctx context.Context) ([]Device, error) {
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
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if sess, ok := s.conns[device.UUID]; ok {
		if sess.Alive() {
			return sess, nil
		}
		if err := sess.Close(); err != nil {
			slog.Warn("chromecast session close failed", slog.Any("err", err))
		}
		delete(s.conns, device.UUID)
	}

	sess, err := s.factory.Connect(*device)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", device.Name, err)
	}
	s.conns[device.UUID] = sess
	return sess, nil
}

func (s *Service) Play(_ context.Context, uuid, mediaURL, title string, contentType string) error {
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

func (s *Service) Pause(_ context.Context, uuid string) error {
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

func (s *Service) Unpause(_ context.Context, uuid string) error {
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

func (s *Service) Stop(_ context.Context, uuid string) error {
	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}

	s.connMu.Lock()
	defer s.connMu.Unlock()

	if sess, ok := s.conns[device.UUID]; ok {
		stopErr := sess.StopMedia()
		if err := sess.Close(); err != nil {
			slog.Warn("chromecast session close failed", slog.Any("err", err))
		}
		delete(s.conns, device.UUID)
		if stopErr != nil {
			return fmt.Errorf("stop: %w", stopErr)
		}
	}
	return nil
}

func (s *Service) Seek(_ context.Context, uuid string, positionSec int) error {
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

func (s *Service) SetVolume(_ context.Context, uuid string, level float32) error {
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

func (s *Service) Status(_ context.Context, uuid string) (map[string]any, error) {
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
	seen := make(map[string]struct{})
	for entry := range entryChan {
		if _, ok := seen[entry.GetUUID()]; ok {
			continue
		}
		seen[entry.GetUUID()] = struct{}{}
		devices = append(devices, domaincast.Device{
			UUID: entry.GetUUID(),
			Name: entry.GetName(),
			Addr: entry.GetAddr(),
			Port: entry.GetPort(),
		})
	}
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
