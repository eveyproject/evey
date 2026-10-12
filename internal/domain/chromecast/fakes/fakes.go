package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/eveyproject/evey/internal/domain/chromecast"
)

type FakeDiscoverer struct {
	Devices []chromecast.Device
	Err     error
	Calls   int
	mu      sync.Mutex
}

func (f *FakeDiscoverer) Discover(_ context.Context) ([]chromecast.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]chromecast.Device, len(f.Devices))
	copy(out, f.Devices)
	return out, nil
}

type FakeSubnetProber struct {
	Devices []chromecast.Device
	Err     error
	Calls   int
	LastIP  string
	mu      sync.Mutex
}

func (f *FakeSubnetProber) ProbeSubnet(_ context.Context, clientIP string) ([]chromecast.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	f.LastIP = clientIP
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]chromecast.Device, len(f.Devices))
	copy(out, f.Devices)
	return out, nil
}

type FakeSession struct {
	Loaded       string
	ContentType  string
	AppLaunched  string
	AppMediaURL  string
	Paused       bool
	Unpaused     bool
	Stopped      bool
	SeekedTo     float32
	Volume       float32
	StatusReturn chrmStatus
	AliveFlag    bool
	Closed       bool

	LoadErr    error
	LoadAppErr error
	PauseErr   error
	UnpauseErr error
	StopErr    error
	SeekErr    error
	VolumeErr  error
	StatusErr  error
}

type chrmStatus = chromecast.MediaStatus

func NewFakeSession() *FakeSession {
	return &FakeSession{AliveFlag: true}
}

func (f *FakeSession) Load(mediaURL, contentType string) error {
	if f.LoadErr != nil {
		return f.LoadErr
	}
	f.Loaded = mediaURL
	f.ContentType = contentType
	return nil
}

func (f *FakeSession) LoadApp(appID, mediaURL string) error {
	if f.LoadAppErr != nil {
		return f.LoadAppErr
	}
	f.AppLaunched = appID
	f.AppMediaURL = mediaURL
	return nil
}

func (f *FakeSession) Pause() error {
	if f.PauseErr != nil {
		return f.PauseErr
	}
	f.Paused = true
	return nil
}

func (f *FakeSession) Unpause() error {
	if f.UnpauseErr != nil {
		return f.UnpauseErr
	}
	f.Unpaused = true
	return nil
}

func (f *FakeSession) StopMedia() error {
	if f.StopErr != nil {
		return f.StopErr
	}
	f.Stopped = true
	return nil
}

func (f *FakeSession) SeekToTime(positionSec float32) error {
	if f.SeekErr != nil {
		return f.SeekErr
	}
	f.SeekedTo = positionSec
	return nil
}

func (f *FakeSession) SetVolume(level float32) error {
	if f.VolumeErr != nil {
		return f.VolumeErr
	}
	f.Volume = level
	return nil
}

func (f *FakeSession) Status() (chromecast.MediaStatus, error) {
	if f.StatusErr != nil {
		return chromecast.MediaStatus{}, f.StatusErr
	}
	return f.StatusReturn, nil
}

func (f *FakeSession) Alive() bool { return f.AliveFlag }

func (f *FakeSession) Close() error {
	f.Closed = true
	return nil
}

type FakeSessionFactory struct {
	Sessions map[string]*FakeSession
	Err      error
	Calls    int
	mu       sync.Mutex
}

func NewFakeSessionFactory() *FakeSessionFactory {
	return &FakeSessionFactory{Sessions: make(map[string]*FakeSession)}
}

func (f *FakeSessionFactory) Connect(device chromecast.Device) (chromecast.CastSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	if f.Err != nil {
		return nil, f.Err
	}
	if s, ok := f.Sessions[device.UUID]; ok {
		return s, nil
	}
	s := NewFakeSession()
	f.Sessions[device.UUID] = s
	return s, nil
}

// ErrNotConnected sentinel for tests.
var ErrNotConnected = errors.New("not connected")
