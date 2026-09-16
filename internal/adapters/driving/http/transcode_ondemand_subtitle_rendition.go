package httpapi

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// The packager declares one segmented-WebVTT rendition per TEXT subtitle
// track of the container, in track order (image tracks are skipped, external
// files are not in the container). The pickers count every subtitle stream,
// so the two numberings are reconciled here, the way audio renditions are.
var extXMediaSubtitles = regexp.MustCompile(`(?m)^#EXT-X-MEDIA:TYPE=SUBTITLES,(.*)$`)

type planSubtitleRendition struct {
	name   string
	lang   string
	uri    string
	forced bool
}

func parsePlanSubtitleRenditions(master string) []planSubtitleRendition {
	var out []planSubtitleRendition
	for _, m := range extXMediaSubtitles.FindAllStringSubmatch(master, -1) {
		out = append(out, planSubtitleRendition{
			name:   attrOf(m[1], "NAME"),
			lang:   attrOf(m[1], "LANGUAGE"),
			uri:    attrOf(m[1], "URI"),
			forced: strings.Contains(m[1], "FORCED=YES"),
		})
	}
	return out
}

// planSubtitleRenditionsFor reads the plan's master and maps its subtitle
// renditions onto the source's subtitle ordinals. Nil when the plan has none
// or cannot render its master - the extraction route still serves them.
func planSubtitleRenditionsFor(ctx context.Context, plan apptranscoding.OnDemandPlan, streams []*domainlibrary.MediaStream) []apptranscoding.SubtitleRendition {
	if plan == nil {
		return nil
	}
	data, _, err := plan.Resource(ctx, "master.m3u8")
	if err != nil {
		return nil
	}
	return mapPlanSubtitleRenditions(parsePlanSubtitleRenditions(string(data)), streams)
}

func mapPlanSubtitleRenditions(rends []planSubtitleRendition, streams []*domainlibrary.MediaStream) []apptranscoding.SubtitleRendition {
	if len(rends) == 0 {
		return nil
	}
	var out []apptranscoding.SubtitleRendition
	next := 0
	idx := -1
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.StreamTypeSubtitle {
			continue
		}
		idx++
		if s.IsExternal || apptranscoding.IsImageSubtitle(s.Codec) {
			continue // not a packager rendition
		}
		if next >= len(rends) {
			break
		}
		r := rends[next]
		if !renditionIsTrack(planAudioRendition{name: r.name, lang: r.lang}, s) {
			continue
		}
		next++
		name, _, _ := strings.Cut(r.uri, "?")
		if name == "" {
			continue
		}
		out = append(out, apptranscoding.SubtitleRendition{
			TrackIdx:     idx,
			Name:         s.Title,
			LanguageCode: s.Language,
			URI:          name,
			Forced:       r.forced || s.IsForced,
		})
	}
	return out
}

// tagPlanMasterSubtitles adds X-EVEY-TRACK to the packager's own master (the
// pure copy path) so a player can tie each rendition to the picker's ordinal
// like on the Evey-built masters, and turns the packager's DEFAULT/AUTOSELECT
// off: the app picks the track, a player-side auto-selection of the forced
// rendition would draw next to it.
func tagPlanMasterSubtitles(master string, rends []apptranscoding.SubtitleRendition) string {
	if len(rends) == 0 {
		return master
	}
	byURI := make(map[string]int, len(rends))
	for _, r := range rends {
		byURI[r.URI] = r.TrackIdx
	}
	return extXMediaSubtitles.ReplaceAllStringFunc(master, func(line string) string {
		uri, _, _ := strings.Cut(attrOf(line, "URI"), "?")
		idx, ok := byURI[uri]
		if !ok {
			return line
		}
		tag := `,X-EVEY-TRACK="` + strconv.Itoa(idx) + `"`
		line = strings.ReplaceAll(line, "DEFAULT=YES", "DEFAULT=NO")
		line = strings.ReplaceAll(line, "AUTOSELECT=YES", "AUTOSELECT=NO")
		// Before URI= so the attribute list stays readable; hls.js and
		// ExoPlayer parse the list as a set, order is cosmetic.
		if j := strings.Index(line, ",URI="); j >= 0 {
			return line[:j] + tag + line[j:]
		}
		return line + tag
	})
}
