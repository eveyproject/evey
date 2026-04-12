package chromecast

import "context"

// Device describes a chromecast device discovered on the network.
type Device struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
}

// MediaStatus is a provider-agnostic snapshot of the current media session.
type MediaStatus struct {
	App         string
	PlayerState string
	CurrentTime float32
	Duration    float32
	Volume      float32
	Muted       bool
}

// Discoverer finds cast devices on the local network.
type Discoverer interface {
	Discover(ctx context.Context) ([]Device, error)
}

// CastSession represents a live connection to a single cast device.
type CastSession interface {
	Load(mediaURL, contentType string) error
	Pause() error
	Unpause() error
	StopMedia() error
	SeekToTime(positionSec float32) error
	SetVolume(level float32) error
	Status() (MediaStatus, error)
	Alive() bool
	Close() error
}

// SessionFactory opens a cast session to a given device.
type SessionFactory interface {
	Connect(device Device) (CastSession, error)
}
