package chromecast

import "context"

// Device describes a chromecast device discovered on the network.
type Device struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
	// Model is the receiver's model name from its mDNS record (TXT md,
	// "Chromecast Ultra", "Google TV Streamer"...), the one capability hint a
	// sender gets before it talks to the device: the web sender reads the
	// shared cast profile table with it. Empty for a device the subnet probe
	// found (TCP 8009 answers, no record) - the sender then negotiates on the
	// floor profile.
	Model string `json:"model,omitempty"`
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

// SubnetProber finds cast devices by probing a client's subnet directly
// (unicast), for deployments where multicast discovery cannot work - an mDNS
// query from a Docker bridge network never reaches the LAN, so a server that
// could perfectly well drive a Chromecast reports an empty device list.
type SubnetProber interface {
	// ProbeSubnet scans the /24 around clientIP for devices. clientIP must be
	// a private (RFC1918) IPv4 address; implementations return nil otherwise.
	ProbeSubnet(ctx context.Context, clientIP string) ([]Device, error)
}

// CastSession represents a live connection to a single cast device.
type CastSession interface {
	Load(mediaURL, contentType string) error
	// LoadApp launches the given receiver app with mediaURL as the LOAD
	// contentId. Implementations return once the launch outcome is known,
	// never blocking until playback ends.
	LoadApp(appID, mediaURL string) error
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
