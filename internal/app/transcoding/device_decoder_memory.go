package transcoding

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// DeviceDecoderMemory remembers, per user and device, the audio codecs the
// app's own decoder (ac3go) passed its launch check for, as the app reports it
// in its ac3go_check beacon. A TV app checks that decoder in the background
// when its player opens, and its first stream negotiation after a cold start
// can leave before the check ends: it then declares the baseline only, and
// an AC-3 film it plays as it is gets its audio transcoded. The app builds in
// the store cannot be changed for days, so the server completes that first
// declaration from what the same device reported before.
//
// Only the stream verdict reads it (GetStreamInfo), never the HLS URLs: a
// master and its segments re-derive one decision from their own query, and a
// memory that moved between two of them would split it.
type DeviceDecoderMemory struct {
	mu  sync.Mutex
	m   map[deviceKey]rememberedDecoders
	now func() time.Time
}

type deviceKey struct{ userID, device string }

type rememberedDecoders struct {
	codecs []string
	at     time.Time
}

// deviceDecoderTTL bounds how long a check is trusted without a new one.
const deviceDecoderTTL = 30 * 24 * time.Hour

// maxRememberedDevices bounds the map: past it the oldest entry goes.
const maxRememberedDevices = 4096

// NewDeviceDecoderMemory returns an empty memory.
func NewDeviceDecoderMemory() *DeviceDecoderMemory {
	return &DeviceDecoderMemory{m: make(map[deviceKey]rememberedDecoders), now: time.Now}
}

// ObservePlayerEvent records what a player beacon says about the app's
// decoder. A passed check is remembered; a blocked check, a decoder that fell
// behind, played silence or died while decoding is forgotten at once.
func (d *DeviceDecoderMemory) ObservePlayerEvent(userID, device, eventType, details string) {
	if d == nil || userID == "" || device == "" {
		return
	}
	key := deviceKey{userID, device}
	switch eventType {
	case "ac3go_check":
		fields := beaconFields(details)
		codecs := splitCodecs(fields["declared"])
		if fields["blocked"] != "no" || len(codecs) == 0 {
			d.forget(key)
			return
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		if _, ok := d.m[key]; !ok && len(d.m) >= maxRememberedDevices {
			d.evictOldestLocked()
		}
		d.m[key] = rememberedDecoders{codecs: codecs, at: d.now()}
	case "ac3go_too_slow_error", "ac3go_silence_error", "ac3go_abnormal_exit":
		d.forget(key)
	}
}

// Complete returns declared with the remembered codecs of this device added,
// and the codecs it added. It adds nothing when the device already declares
// one of them: that declaration came after its check, and whatever it left
// out it left out on purpose.
func (d *DeviceDecoderMemory) Complete(userID, device string, declared []string) (out, added []string) {
	if d == nil || userID == "" || device == "" || len(declared) == 0 {
		return declared, nil
	}
	d.mu.Lock()
	entry, ok := d.m[deviceKey{userID, device}]
	if ok && d.now().Sub(entry.at) > deviceDecoderTTL {
		delete(d.m, deviceKey{userID, device})
		ok = false
	}
	d.mu.Unlock()
	if !ok {
		return declared, nil
	}
	for _, c := range entry.codecs {
		if slices.Contains(declared, c) {
			return declared, nil
		}
	}
	out = append(slices.Clone(declared), entry.codecs...)
	return out, slices.Clone(entry.codecs)
}

func (d *DeviceDecoderMemory) forget(key deviceKey) {
	d.mu.Lock()
	delete(d.m, key)
	d.mu.Unlock()
}

func (d *DeviceDecoderMemory) evictOldestLocked() {
	var oldest deviceKey
	var at time.Time
	first := true
	for k, v := range d.m {
		if first || v.at.Before(at) {
			oldest, at, first = k, v.at, false
		}
	}
	delete(d.m, oldest)
}

// beaconFields reads the "key=value key=value" details of a beacon.
func beaconFields(details string) map[string]string {
	out := make(map[string]string)
	for f := range strings.FieldsSeq(details) {
		if k, v, ok := strings.Cut(f, "="); ok {
			out[k] = v
		}
	}
	return out
}

func splitCodecs(v string) []string {
	var out []string
	for c := range strings.SplitSeq(v, ",") {
		if c = strings.TrimSpace(strings.ToLower(c)); c != "" && c != "none" {
			out = append(out, c)
		}
	}
	return out
}
