package loadtest

import (
	"bufio"
	"bytes"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// variant is one #EXT-X-STREAM-INF entry of a master playlist.
type variant struct {
	bandwidth int
	uri       *url.URL
}

// audioRendition is one #EXT-X-MEDIA:TYPE=AUDIO entry: a separate playlist the
// player must pull ALONGSIDE the video.
//
// Ignoring these is not a small omission. Evey's direct-play path demuxes the
// source - the video and each audio track are separate renditions - so a real
// player fetches TWO segments per moment, from two playlists. A simulated viewer
// that only pulls the video costs the server roughly half what a real one does,
// and never exercises the packager's single-walk path at all. The measurement
// would be of a client nobody runs.
type audioRendition struct {
	groupID   string
	name      string
	isDefault bool
	uri       *url.URL
}

// masterPlaylist is what a player gets from the master: the video ladder, and
// the audio renditions it must pull in parallel.
type masterPlaylist struct {
	variants []variant
	audio    []audioRendition
}

// parseMasterPlaylist reads an HLS master playlist and resolves every variant
// and audio-rendition URI against base (the URL the playlist was fetched from).
func parseMasterPlaylist(body []byte, base *url.URL) (masterPlaylist, error) {
	var out masterPlaylist
	scanner := bufio.NewScanner(bytes.NewReader(body))
	var pendingBandwidth int
	havePending := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA:"):
			if !strings.Contains(line, "TYPE=AUDIO") {
				continue // subtitles and closed captions: a player pulls them only when enabled
			}
			raw := parseAttrString(line, "URI")
			if raw == "" {
				continue // an audio group carried inside the video segments has no URI
			}
			ref, err := url.Parse(raw)
			if err != nil {
				return masterPlaylist{}, fmt.Errorf("loadtest: parse audio rendition uri %q: %w", raw, err)
			}
			out.audio = append(out.audio, audioRendition{
				groupID:   parseAttrString(line, "GROUP-ID"),
				name:      parseAttrString(line, "NAME"),
				isDefault: strings.Contains(line, "DEFAULT=YES"),
				uri:       base.ResolveReference(ref),
			})
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			pendingBandwidth = parseAttrInt(line, "BANDWIDTH")
			havePending = true
		case line == "" || strings.HasPrefix(line, "#"):
			// comment or blank: not a URI line.
		default:
			if !havePending {
				continue
			}
			ref, err := url.Parse(line)
			if err != nil {
				return masterPlaylist{}, fmt.Errorf("loadtest: parse variant uri %q: %w", line, err)
			}
			out.variants = append(out.variants, variant{bandwidth: pendingBandwidth, uri: base.ResolveReference(ref)})
			havePending = false
		}
	}
	if err := scanner.Err(); err != nil {
		return masterPlaylist{}, fmt.Errorf("loadtest: scan master playlist: %w", err)
	}
	if len(out.variants) == 0 {
		return masterPlaylist{}, fmt.Errorf("loadtest: master playlist has no variants")
	}
	return out, nil
}

// pickAudioRendition returns the audio track a player would actually play: the
// one flagged DEFAULT, else the first. A player pulls exactly ONE - never every
// language - which is precisely why the packager cannot wait for all of them to
// be collected before releasing a window.
func pickAudioRendition(renditions []audioRendition) (audioRendition, bool) {
	if len(renditions) == 0 {
		return audioRendition{}, false
	}
	for _, r := range renditions {
		if r.isDefault {
			return r, true
		}
	}
	return renditions[0], true
}

// pickMedianVariant returns the middle-quality variant of the ladder, sorted
// by bandwidth ascending, mirroring a real ABR player which starts mid-ladder
// rather than at the top.
func pickMedianVariant(variants []variant) variant {
	sorted := make([]variant, len(variants))
	copy(sorted, variants)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].bandwidth < sorted[j].bandwidth })
	return sorted[len(sorted)/2]
}

// segmentDefaultMs is used when a variant playlist doesn't declare
// #EXT-X-TARGETDURATION.
const segmentDefaultMs = 3000

// parseVariantPlaylist reads an HLS media playlist, returning the declared
// target duration (ms) and the resolved segment URLs in playback order.
// segmentDurationsMs reads each segment's own #EXTINF duration, in playlist
// order, so a fetch can be judged against the length of the segment it
// fetched. A segment without an EXTINF gets 0 (unknown).
func segmentDurationsMs(body []byte) []int {
	var out []int
	pending := 0
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			raw := strings.TrimPrefix(line, "#EXTINF:")
			if i := strings.IndexByte(raw, ','); i >= 0 {
				raw = raw[:i]
			}
			if secs, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && secs > 0 {
				pending = int(secs * 1000)
			}
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			out = append(out, pending)
			pending = 0
		}
	}
	return out
}

func parseVariantPlaylist(body []byte, base *url.URL) (targetDurationMs int, segments []*url.URL, err error) {
	targetDurationMs = segmentDefaultMs
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			// The HLS spec mandates an integer number of seconds here, but
			// this parser tolerates a decimal too (tests use sub-second
			// durations to keep real-time pacing fast without changing the
			// pacing logic itself).
			raw := strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:")
			if secs, convErr := strconv.ParseFloat(strings.TrimSpace(raw), 64); convErr == nil {
				targetDurationMs = int(secs * 1000)
			}
		case line == "" || strings.HasPrefix(line, "#"):
			// comment, EXTINF, or other tag: not a URI line.
		default:
			ref, parseErr := url.Parse(line)
			if parseErr != nil {
				return 0, nil, fmt.Errorf("loadtest: parse segment uri %q: %w", line, parseErr)
			}
			segments = append(segments, base.ResolveReference(ref))
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return 0, nil, fmt.Errorf("loadtest: scan variant playlist: %w", scanErr)
	}
	if len(segments) == 0 {
		return 0, nil, fmt.Errorf("loadtest: variant playlist has no segments")
	}
	return targetDurationMs, segments, nil
}

// parseAttrInt extracts an integer attribute (e.g. BANDWIDTH=1234000) from an
// HLS tag line. Returns 0 if absent or unparsable.
func parseAttrInt(line, attr string) int {
	idx := strings.Index(line, attr+"=")
	if idx < 0 {
		return 0
	}
	rest := line[idx+len(attr)+1:]
	end := strings.IndexAny(rest, ",\r\n")
	if end >= 0 {
		rest = rest[:end]
	}
	v, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return 0
	}
	return v
}

// parseAttrString extracts a quoted string attribute (e.g. URI="audio.m3u8")
// from an HLS tag line. Returns "" if absent.
func parseAttrString(line, attr string) string {
	idx := strings.Index(line, attr+"=")
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(attr)+1:]
	if strings.HasPrefix(rest, `"`) {
		if end := strings.Index(rest[1:], `"`); end >= 0 {
			return rest[1 : 1+end]
		}
		return ""
	}
	if end := strings.IndexAny(rest, ",\r\n"); end >= 0 {
		return rest[:end]
	}
	return rest
}
