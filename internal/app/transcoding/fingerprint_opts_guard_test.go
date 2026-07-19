package transcoding

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The DecisionFingerprint describes the BYTES a session will produce; the two
// HLSProcessOpts literals are what actually reaches ffmpeg. Nothing links them
// mechanically, so a new fingerprint field can be hashed (forking the cache)
// yet never threaded to the encoder - the two byte-streams then differ from
// what their hashes promise. This guard forces every field into one of two
// explicit buckets, and checks the threaded ones really are read at a mapping
// site. It cannot prove the mapping is CORRECT, only that no field is
// forgotten in silence.
//
// Adding a field? Put it in the right map below.
var (
	// Read by a mapping site and passed to ffmpeg.
	fingerprintEncoderInputs = map[string]string{
		"VideoCodec":          "opts.VideoCodec",
		"AudioCodec":          "opts.AudioCodec",
		"AudioTrack":          "opts.AudioTrackIndex",
		"SubtitleBurnInTrack": "opts.SubtitleTrackIndex",
		"SubtitleBurnInCodec": "opts.SubtitleCodec",
		"HDRTonemap":          "opts.ToneMap",
		"AudioNormalize":      "opts.AudioNormalize",
		"MaxAudioChannels":    "opts.MaxAudioChannels",
		"ForceSidecarSubs":    "opts.ForceSidecarSubs",
		"UseFMP4":             "opts.UseFMP4",
		"AudioOffsetMs":       "opts.AudioUserOffsetSec (ms -> s)",
		"HWAccel":             "opts.HWAccel",
		"HWDevice":            "opts.HWDevice",
		"HWFeatures":          "opts.HWFeatures",
		"HWTonemap":           "opts.HWTonemap",
		"DeinterlaceAll":      "opts.DeinterlaceAll",
		"SourceSAR":           "opts.SourceSAR",
		"SourceVideoCodec":    "opts.SourceVideoCodec (copy path -tag:v)",
	}
	// Part of the cache identity but NOT an encoder input - each with the
	// reason it legitimately stops at the fingerprint.
	fingerprintNonEncoderFields = map[string]string{
		"MediaID":    "cache key only; the source is resolved from SourcePath",
		"SourcePath": "reaches ffmpeg as opts.FilePath, resolved by the caller",
		"AudioCopy":  "selects the audio branch via fp.AudioDecision(), not a raw field",
		"Profiles":   "converted by profilesToDomain into opts.Profiles",
	}
)

func TestDecisionFingerprint_EveryFieldIsRoutedOrExempt(t *testing.T) {
	for f := range reflect.TypeFor[DecisionFingerprint]().Fields() {
		name := f.Name
		_, threaded := fingerprintEncoderInputs[name]
		_, exempt := fingerprintNonEncoderFields[name]
		switch {
		case threaded && exempt:
			t.Errorf("%s is declared both an encoder input and exempt - pick one", name)
		case !threaded && !exempt:
			t.Errorf("DecisionFingerprint.%s is in neither map: declare where it reaches ffmpeg, "+
				"or why it stops at the fingerprint (see this file's header)", name)
		}
	}
}

func TestDecisionFingerprint_EncoderInputsAreReadAtAMappingSite(t *testing.T) {
	sites := []string{"segment_orchestrator.go", "software_producer.go"}
	var sources []string
	for _, f := range sites {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v (mapping site moved? update this guard)", f, err)
		}
		sources = append(sources, string(b))
	}
	for field := range fingerprintEncoderInputs {
		// `fp.Field` / `fp.Field()` as written in the literals.
		re := regexp.MustCompile(`\bfp\.` + regexp.QuoteMeta(field) + `\b`)
		if !slices.ContainsFunc(sources, re.MatchString) {
			t.Errorf("DecisionFingerprint.%s is declared an encoder input (%s) but no mapping site reads fp.%s - "+
				"the encoder never receives it, so the segments won't match what the hash promises. Sites: %s",
				field, fingerprintEncoderInputs[field], field, strings.Join(sites, ", "))
		}
	}
}
