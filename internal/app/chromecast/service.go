package chromecast

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/dns"
)

const (
	cacheTTL        = 60 * time.Second
	discoveryTimeout = 5 * time.Second
)

type Device struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
}

type Service struct {
	mu       sync.RWMutex
	devices  []Device
	lastScan time.Time
	baseURL  string

	connMu sync.Mutex
	conns  map[string]*application.Application
}

func New(baseURL string) *Service {
	return &Service{
		baseURL: baseURL,
		conns:   make(map[string]*application.Application),
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

	devices, err := s.discover(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.devices = devices
	s.lastScan = time.Now()
	s.mu.Unlock()

	return devices, nil
}

func (s *Service) discover(ctx context.Context) ([]Device, error) {
	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	entryChan, err := dns.DiscoverCastDNSEntries(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mdns discover: %w", err)
	}

	var devices []Device
	seen := make(map[string]struct{})
	for entry := range entryChan {
		if _, ok := seen[entry.GetUUID()]; ok {
			continue
		}
		seen[entry.GetUUID()] = struct{}{}
		devices = append(devices, Device{
			UUID: entry.GetUUID(),
			Name: entry.GetName(),
			Addr: entry.GetAddr(),
			Port: entry.GetPort(),
		})
	}
	slog.Info("chromecast: discovery complete", slog.Int("count", len(devices)))
	return devices, nil
}

func (s *Service) findDevice(uuid string) (*Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.devices {
		if d.UUID == uuid {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("device %q not found", uuid)
}

func (s *Service) getOrConnect(device *Device) (*application.Application, error) {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if app, ok := s.conns[device.UUID]; ok {
		if err := app.Update(); err == nil {
			return app, nil
		}
		app.Close(false)
		delete(s.conns, device.UUID)
	}

	app := application.NewApplication(
		application.WithCacheDisabled(true),
		application.WithDeviceNameOverride(device.Name),
	)
	if err := app.Start(device.Addr, device.Port); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", device.Name, err)
	}

	s.conns[device.UUID] = app
	return app, nil
}

func (s *Service) Play(ctx context.Context, uuid, mediaURL, title string, contentType string) error {
	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}

	app, err := s.getOrConnect(device)
	if err != nil {
		return err
	}

	if contentType == "" {
		contentType = "video/mp4"
	}

	if err := app.Load(mediaURL, 0, contentType, false, false, false); err != nil {
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
	app, err := s.getOrConnect(device)
	if err != nil {
		return err
	}
	return app.Pause()
}

func (s *Service) Unpause(_ context.Context, uuid string) error {
	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}
	app, err := s.getOrConnect(device)
	if err != nil {
		return err
	}
	return app.Unpause()
}

func (s *Service) Stop(_ context.Context, uuid string) error {
	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}

	s.connMu.Lock()
	defer s.connMu.Unlock()

	if app, ok := s.conns[device.UUID]; ok {
		err := app.StopMedia()
		app.Close(true)
		delete(s.conns, device.UUID)
		if err != nil {
			return fmt.Errorf("stop: %w", err)
		}
	}
	return nil
}

func (s *Service) Seek(_ context.Context, uuid string, positionSec int) error {
	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}
	app, err := s.getOrConnect(device)
	if err != nil {
		return err
	}
	return app.SeekToTime(float32(positionSec))
}

func (s *Service) SetVolume(_ context.Context, uuid string, level float32) error {
	device, err := s.findDevice(uuid)
	if err != nil {
		return err
	}
	app, err := s.getOrConnect(device)
	if err != nil {
		return err
	}
	return app.SetVolume(level)
}

func (s *Service) Status(_ context.Context, uuid string) (map[string]interface{}, error) {
	device, err := s.findDevice(uuid)
	if err != nil {
		return nil, err
	}
	app, err := s.getOrConnect(device)
	if err != nil {
		return nil, err
	}
	castApp, media, vol := app.Status()
	result := map[string]interface{}{
		"device": device.Name,
	}
	if castApp != nil {
		result["app"] = castApp.DisplayName
	}
	if media != nil {
		result["playerState"] = media.PlayerState
		result["currentTime"] = media.CurrentTime
		result["duration"] = media.Media.Duration
	}
	if vol != nil {
		result["volume"] = vol.Level
		result["muted"] = vol.Muted
	}
	return result, nil
}

func (s *Service) Close() {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	for uuid, app := range s.conns {
		app.Close(false)
		delete(s.conns, uuid)
	}
}
