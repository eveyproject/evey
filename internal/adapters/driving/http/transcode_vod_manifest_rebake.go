package httpapi

// transcode_vod_manifest_rebake.go rewrites segment/init URIs inside HLS
// and DASH manifests to carry the per-session suffix.

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

// rebakeManifestURIs appends the request's auth + decision query suffix to
// every URI in an HLS playlist: bare segment lines, plus the URI="..." of any
// tag that carries one (#EXT-X-MAP init, #EXT-X-KEY AES key, #EXT-X-MEDIA
// audio/subtitle renditions, #EXT-X-STREAM-INF children in a master). No-op on
// empty suffix or on URIs that already carry a query.
func rebakeManifestURIs(manifest, suffix string) string {
	if suffix == "" {
		return manifest
	}
	lines := strings.Split(manifest, "\n")
	for i, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "#") && strings.Contains(ln, "URI=\""):
			lines[i] = addSuffixToURIAttr(ln, suffix)
		case ln != "" && !strings.HasPrefix(ln, "#") && !strings.Contains(ln, "?"):
			lines[i] = ln + suffix
		}
	}
	return strings.Join(lines, "\n")
}

// addSuffixToURIAttr appends suffix inside the first URI="..." attribute of an
// HLS tag line, unless the value is empty or already has a query.
func addSuffixToURIAttr(line, suffix string) string {
	const marker = "URI=\""
	i := strings.Index(line, marker)
	if i < 0 {
		return line
	}
	start := i + len(marker)
	rel := strings.IndexByte(line[start:], '"')
	if rel < 0 {
		return line
	}
	val := line[start : start+rel]
	if val == "" || strings.Contains(val, "?") {
		return line
	}
	return line[:start] + val + suffix + line[start+rel:]
}

// dashURIAttrRE matches the segment/init template attributes of a DASH manifest
// whose value carries no query yet.
var dashURIAttrRE = regexp.MustCompile(`(media|initialization)="([^"?]*)"`)

// rebakeDASHManifestURIs appends suffix to the media/initialization template
// URIs of a DASH manifest so a cookie-less client (Chromecast) authenticates
// its segment/init requests via the query token. The suffix lands in an XML
// attribute: its '&' are escaped, or the manifest is malformed XML (Media3
// refused it outright: ERROR_CODE_PARSING_MANIFEST_MALFORMED). Taken
// verbatim, never as a replacement template ('$' in a client's query).
func rebakeDASHManifestURIs(manifest, suffix string) string {
	if suffix == "" {
		return manifest
	}
	esc := html.EscapeString(suffix)
	return dashURIAttrRE.ReplaceAllStringFunc(manifest, func(m string) string {
		sub := dashURIAttrRE.FindStringSubmatch(m)
		return sub[1] + `="` + sub[2] + esc + `"`
	})
}

// stampPlanSourceURIs adds the plan's SourceID to every packager URI (od_*) of
// an HLS master Evey composed itself (hybrid, wasm audio), whose od_ playlists
// the packager's rebake never sees. Other URIs are left alone.
func stampPlanSourceURIs(manifest, sourceID string) string {
	if sourceID == "" {
		return manifest
	}
	param := planSourceParam + "=" + url.QueryEscape(sourceID)
	stamp := func(uri string) string {
		if !strings.HasPrefix(uri, onDemandPrefix) {
			return uri
		}
		if strings.Contains(uri, "?") {
			return uri + "&" + param
		}
		return uri + "?" + param
	}
	lines := strings.Split(manifest, "\n")
	for i, ln := range lines {
		if !strings.HasPrefix(ln, "#") {
			lines[i] = stamp(ln)
			continue
		}
		const marker = "URI=\""
		start := strings.Index(ln, marker)
		if start < 0 {
			continue
		}
		start += len(marker)
		end := strings.IndexByte(ln[start:], '"')
		if end < 0 {
			continue
		}
		lines[i] = ln[:start] + stamp(ln[start:start+end]) + ln[start+end:]
	}
	return strings.Join(lines, "\n")
}
