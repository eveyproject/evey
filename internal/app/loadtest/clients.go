package loadtest

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ScenarioClients is the prefix of the client-mix scenarios: each simulated
// viewer declares what one family of real players declares (codecs,
// containers, HDR, channels) and the server picks its path - copy, remux or
// encode - exactly as it would for that player. "clients:history" takes the
// mix from the server's own recent playbacks, "clients:tv=40,mobile=40,web=20"
// states it.
const ScenarioClients = "clients"

// ClientMixHistory asks for the mix of the server's own playbacks.
const ClientMixHistory = "history"

// DefaultClientMix is the mix a history run falls back to when the server has
// too few playbacks to say: a household that watches mostly on a TV and phones.
const DefaultClientMix = "tv=40,mobile=40,web-chrome=20"

// minHistorySessions is how many playbacks a history mix needs before its
// proportions mean anything.
const minHistorySessions = 20

// clientProfile is the query a family of players puts on its master playlist,
// copied from what the shipped clients send (frontend/packages/client: web/
// hlsCodecParams.ts + codec.ts, native/playSession.ts). Where a real player
// asks its device (a browser's HEVC, a phone's HDR panel), the profile takes
// the common case and says so.
type clientProfile struct {
	params url.Values
}

var clientProfiles = map[string]clientProfile{
	// Chrome/Edge/Firefox on a PC: no HEVC without hardware support, Dolby
	// audio through the ac3go wasm decoder (wasmAudio, never audioCodecs), an
	// SDR screen, stereo output.
	"web-chrome": {url.Values{
		"videoCodecs": {"h264,vp9,av1"}, "audioCodecs": {"aac,mp3,opus,flac"}, "containers": {"mp4,webm"},
		"fmp4AudioPassthrough": {"mp3,opus"}, "wasmAudio": {"1"}, "maxAudioChannels": {"2"},
	}},
	// Safari on a Mac or an iPhone: HEVC and Dolby audio natively, AVPlayer
	// (no fMP4 passthrough), SDR assumed.
	"web-safari": {url.Values{
		"videoCodecs": {"h264,hevc"}, "audioCodecs": {"aac,mp3,ac3,eac3,flac"}, "containers": {"mp4"},
	}},
	// The Android app on a phone: Matroska demuxed natively, Dolby audio
	// through ac3go, an SDR panel assumed.
	"mobile": {url.Values{
		"videoCodecs": {"h264,hevc,vp9"}, "audioCodecs": {"aac,mp3,opus,flac,ac3,eac3"}, "containers": {"mp4,mkv,webm,hls"},
	}},
	// The web app in a smart TV's own browser (frontend/apps/tv-web, an LG
	// NetCast/webOS Chrome 120 measured 2026-09-28): no HEVC through MSE, no
	// HDR, a stereo output, no wasm audio decoder.
	"tv-browser": {url.Values{
		"videoCodecs": {"h264,vp9"}, "audioCodecs": {"aac,mp3,opus,flac"}, "containers": {"mp4,webm"},
		"maxAudioChannels": {"2"},
	}},
	// The Android TV app (Google TV, Chromecast with Google TV): an HDR10
	// panel and a 5.1 output.
	"tv": {url.Values{
		"videoCodecs": {"h264,hevc,vp9"}, "audioCodecs": {"aac,mp3,opus,flac,ac3,eac3"}, "containers": {"mp4,mkv,webm,hls"},
		"hdr": {"1"}, "maxAudioChannels": {"6"},
	}},
}

// PlaybackDevice is what a playback recorded about its client: the app's
// device type ("Evey TV") and its device name ("TV Browser").
type PlaybackDevice struct {
	Type, Name string
}

// historyClientFamily maps a playback's recorded device type to the profile
// that plays it.
var historyClientFamily = map[string]string{
	"Evey Web":    "web-chrome",
	"Evey Mobile": "mobile",
	"Evey TV":     "tv",
}

// historyClientByName overrides the type for a device name that plays very
// differently from its type: the TV web app records itself as "Evey TV" but
// plays like a browser, without HEVC or HDR.
var historyClientByName = map[string]string{
	"TV Browser": "tv-browser",
}

func historyProfile(d PlaybackDevice) (string, bool) {
	if p, ok := historyClientByName[d.Name]; ok {
		return p, true
	}
	p, ok := historyClientFamily[d.Type]
	return p, ok
}

// clientViewerPrefix marks a viewerType that plays as a client profile.
const clientViewerPrefix = "client:"

func clientViewerType(profile string) viewerType { return viewerType(clientViewerPrefix + profile) }

// clientProfileOf returns the profile a viewer type plays as, if it is one.
func clientProfileOf(vt viewerType) (clientProfile, bool) {
	name, ok := strings.CutPrefix(string(vt), clientViewerPrefix)
	if !ok {
		return clientProfile{}, false
	}
	p, ok := clientProfiles[name]
	return p, ok
}

// clientShare is one profile's weight in a mix.
type clientShare struct {
	profile string
	weight  int
}

// parseClientMix reads "tv=40,mobile=40,web-chrome=20" into its shares, in
// the order given. Weights are relative, not necessarily percentages.
func parseClientMix(spec string) ([]clientShare, error) {
	var out []clientShare
	for part := range strings.SplitSeq(spec, ",") {
		name, w, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, fmt.Errorf("client mix %q: want profile=weight", part)
		}
		if _, known := clientProfiles[name]; !known {
			return nil, fmt.Errorf("client mix: unknown profile %q", name)
		}
		n, err := strconv.Atoi(w)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("client mix %q: weight must be a whole number >= 0", part)
		}
		if n > 0 {
			out = append(out, clientShare{name, n})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("client mix %q: no profile with a weight", spec)
	}
	return out, nil
}

// clientMixFromHistory turns playback counts by recorded device into a
// mix spec. ok is false when the history is too thin to stand for an
// audience (fewer than minHistorySessions playbacks the profiles can play).
func clientMixFromHistory(byDevice map[PlaybackDevice]int) (spec string, ok bool) {
	weights := map[string]int{}
	total := 0
	for d, n := range byDevice {
		if p, known := historyProfile(d); known && n > 0 {
			weights[p] += n
			total += n
		}
	}
	if total < minHistorySessions {
		return "", false
	}
	names := make([]string, 0, len(weights))
	for p := range weights {
		names = append(names, p)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, p := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", p, weights[p]))
	}
	return strings.Join(parts, ","), true
}

// clientForSlot is the profile of the viewer at a 1-based audience rank:
// each rank goes to the profile furthest behind its share, so any audience
// size holds the mix within one viewer per profile.
func clientForSlot(mix []clientShare, slot int) string {
	total := 0
	for _, s := range mix {
		total += s.weight
	}
	counts := make([]int, len(mix))
	best := 0
	for rank := 1; rank <= slot; rank++ {
		best = 0
		bestDeficit := -1.0
		for i, s := range mix {
			if d := float64(s.weight)*float64(rank)/float64(total) - float64(counts[i]); d > bestDeficit {
				best, bestDeficit = i, d
			}
		}
		counts[best]++
	}
	return mix[best].profile
}
