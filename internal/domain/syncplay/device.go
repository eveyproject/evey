package syncplay

import "time"

// Device is a first-party evey client (TV app, mobile, another browser
// tab) holding a persistent WebSocket to the server and declared as
// controllable. It is the "receiver" for the Play On feature: the server
// brokers playback commands to it directly, with no third-party casting
// protocol (Chromecast/AirPlay) and no LAN dependency — it works for
// remote clients exactly as for local ones, because everything is relayed
// through the server connection each client already holds.
type Device struct {
	ID          string // client-generated, stable per install
	UserID      string
	Name        string // user-facing, e.g. "Living room TV"
	Type        string // "tv" | "mobile" | "web"
	ConnectedAt time.Time
}

// Device.Type values. Clients send one of these at registration; unknown
// values are accepted as-is so the protocol can grow without a server
// release, but these are the ones the UI knows how to icon/label.
const (
	DeviceTypeTV     = "tv"
	DeviceTypeMobile = "mobile"
	DeviceTypeWeb    = "web"
)

// DeviceRegisterRequest is the first frame a client sends on the device
// WebSocket. It declares the client as a controllable Play On target.
type DeviceRegisterRequest struct {
	Type       string `json:"type"`       // must be "register"
	DeviceID   string `json:"deviceId"`   // stable per install
	Name       string `json:"name"`       // user-facing
	DeviceType string `json:"deviceType"` // see DeviceType* constants
}

// DeviceCommand is a server→device push: a playback instruction the
// target client acts on. Distinct from SyncCommand, which is the
// room-scoped peer-relay protocol for synchronized watch parties — a
// DeviceCommand has a single addressed recipient and no room.
type DeviceCommand struct {
	Type       string `json:"type"`                 // "loadAndPlay" | "play" | "pause" | "seek"
	MediaID    string `json:"mediaId,omitempty"`    // for loadAndPlay
	PositionMs int64  `json:"positionMs,omitempty"` // for loadAndPlay (resume) / seek
}

// DeviceCommand.Type values.
const (
	DeviceCmdLoadAndPlay = "loadAndPlay"
	DeviceCmdPlay        = "play"
	DeviceCmdPause       = "pause"
	DeviceCmdSeek        = "seek"
)
