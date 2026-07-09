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

// parseMasterPlaylist reads an HLS master playlist and resolves every
// variant URI against base (the URL the playlist was fetched from).
func parseMasterPlaylist(body []byte, base *url.URL) ([]variant, error) {
	var variants []variant
	scanner := bufio.NewScanner(bytes.NewReader(body))
	var pendingBandwidth int
	havePending := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
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
				return nil, fmt.Errorf("loadtest: parse variant uri %q: %w", line, err)
			}
			variants = append(variants, variant{bandwidth: pendingBandwidth, uri: base.ResolveReference(ref)})
			havePending = false
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("loadtest: scan master playlist: %w", err)
	}
	if len(variants) == 0 {
		return nil, fmt.Errorf("loadtest: master playlist has no variants")
	}
	return variants, nil
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
