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
	App string
	// AppID is the Cast application id of whatever is running on the device
	// (e.g. "D36A449B" for the Evey receiver). The co-sender keys its "is
	// this still OUR session" check on it: the display name is localized and
	// operator-editable in the Cast console, the id is not.
	AppID       string
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
