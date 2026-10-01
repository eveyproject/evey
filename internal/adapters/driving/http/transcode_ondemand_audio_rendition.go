package httpapi

import (
	"context"
	"regexp"
	"strings"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// Naming the RIGHT audio rendition of an on-demand plan.
//
// The plan's renditions are numbered positionally among the tracks IT kept, and
// it keeps only what it can package - so `audioTrack + 1` is correct only when
// every source audio track survives. It usually does; when it does not, the
// arithmetic does not 404, it silently addresses ANOTHER LANGUAGE.
//
// Prod, 2026-09-09, a 4-track release (TrueHD Atmos 7.1 eng, [VFF] AC3, [VFQ]
// AC3, AC3 eng): the plan published three renditions,
//
//	#EXT-X-MEDIA:...,NAME="[VFF] AC3 5.1",LANGUAGE="fre",URI="od_audio1.m3u8"
//	#EXT-X-MEDIA:...,NAME="[VFQ] AC3 5.1",LANGUAGE="fre",URI="od_audio2.m3u8"
//	#EXT-X-MEDIA:...,NAME="AC3 5.1",URI="od_audio3.m3u8"
//
// so picking the VFQ (audioTrack=2) played od_audio3 - the English track. The
// viewer heard English with "VFQ" ticked in the UI, and nothing anywhere said so.
//
// The fix is to stop deriving the number and READ the plan's own master, which
// publishes NAME and LANGUAGE per rendition. Matching is positional against the
// source's audio tracks, using those attributes as the evidence that a given
// source track did survive into the plan.
//
// This deliberately does NOT decide anything about which tracks a plan should
// carry: a track the plan drops is still served, by the ffmpeg copy or AAC
// rendition, exactly as before. Only the NAMING is corrected.

var extXMediaAudio = regexp.MustCompile(`(?m)^#EXT-X-MEDIA:TYPE=AUDIO,(.*)$`)

// planAudioRendition is one #EXT-X-MEDIA:TYPE=AUDIO entry of a plan's master.
type planAudioRendition struct {
	name string
	lang string
	uri  string
}

// parsePlanAudioRenditions reads the audio renditions a plan publishes, in the
// order the master lists them - which is the order of the source tracks it kept.
func parsePlanAudioRenditions(master string) []planAudioRendition {
	var out []planAudioRendition
	for _, m := range extXMediaAudio.FindAllStringSubmatch(master, -1) {
		out = append(out, planAudioRendition{
			name: attrOf(m[1], "NAME"),
			lang: attrOf(m[1], "LANGUAGE"),
			uri:  attrOf(m[1], "URI"),
		})
	}
	return out
}

// attrOf pulls one quoted attribute out of an EXT-X-MEDIA attribute list.
// Values are quoted-string per RFC 8216 for all three attributes read here.
func attrOf(attrs, key string) string {
	i := strings.Index(attrs, key+`="`)
	if i < 0 {
		return ""
	}
	rest := attrs[i+len(key)+2:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// planRenditionURIFor returns the plan rendition URI carrying the audioTrack-th
// SOURCE audio track, and whether it could be resolved.
//
// Both sequences are in source order, so this walks the source's audio tracks
// and consumes a rendition whenever the two agree on what the track is. A track
// the plan dropped consumes nothing, which is what re-aligns everything after it.
//
// Returns false rather than a guess when the two cannot be aligned: the caller
// then keeps the previous arithmetic, so an unrecognised shape is no worse off
// than before this existed.
func planRenditionURIFor(rends []planAudioRendition, streams []*domainlibrary.MediaStream, audioTrack int) (string, bool) {
	if len(rends) == 0 || audioTrack < 0 {
		return "", false
	}
	next := 0
	idx := -1
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.StreamTypeAudio {
			continue
		}
		idx++
		if next >= len(rends) {
			break
		}
		if !renditionIsTrack(rends[next], s) {
			continue // the plan dropped this source track
		}
		if idx == audioTrack {
			// The published URI carries the plan's OWN query (a cache-busting
			// ?v=<version>); the caller appends the decision query, so hand back
			// the bare resource name.
			name, _, _ := strings.Cut(rends[next].uri, "?")
			if name == "" {
				return "", false
			}
			return name, true
		}
		next++
	}
	return "", false
}

// renditionIsTrack reports whether a published rendition describes this source
// track. The plan copies the track's title into NAME and its language into
// LANGUAGE, so those two are the evidence. A rendition with neither (an
// untitled, language-less track) is accepted on position alone - refusing it
// would strand the very files that need this most.
func renditionIsTrack(r planAudioRendition, s *domainlibrary.MediaStream) bool {
	if r.name == "" && r.lang == "" {
		return true
	}
	if r.name != "" && s.Title != "" {
		return strings.EqualFold(normTrackName(r.name), normTrackName(s.Title))
	}
	if r.lang != "" && s.Language != "" {
		return strings.EqualFold(r.lang, s.Language)
	}
	return true
}

// normTrackName reads a track name the way a viewer does: the packager writes
// a control character of the stored title as its HLS escape ("Português\n"
// against a title ending in a real newline), and runs of whitespace are one
// space. Compared raw, one such title failed to match, and every track after
// it was dropped from the master (Sprite Fright, 2026-10-01).
func normTrackName(s string) string {
	s = strings.NewReplacer(`\n`, " ", `\r`, " ", `\t`, " ", `\"`, `"`).Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// planAudioRenditionsOf reads a plan's master and parses its audio renditions.
// A plan that cannot render its own master yields none, and the caller falls
// back - this is a naming aid, never a gate on serving.
func planAudioRenditionsOf(ctx context.Context, plan apptranscoding.OnDemandPlan) []planAudioRendition {
	if plan == nil {
		return nil
	}
	data, _, err := plan.Resource(ctx, "master.m3u8")
	if err != nil {
		return nil
	}
	return parsePlanAudioRenditions(string(data))
}
