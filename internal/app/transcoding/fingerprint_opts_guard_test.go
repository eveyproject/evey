package transcoding

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"reflect"
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
// KNOWN SCOPE, deliberate on both counts - do not read more into a pass than
// this:
//   - AT-LEAST-ONE, not all-sites. A field satisfies the guard as soon as ONE
//     opts literal reads it. That is the right rule here because the mapping
//     sites legitimately diverge (the audio-only path threads no video field),
//     but it means a field that ought to reach TWO paths and only reaches one
//     still passes. The failure message lists every site so the asymmetry is
//     at least visible when something else breaks.
//   - READ, not correctness. `FilePath: fp.SourcePath` and a field assigned to
//     the wrong opts key look identical here.
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
		// Priced, never encoded: ffmpeg learns the height by opening the file.
		"SourceVideoHeight": "admission input only - VideoCostModel.CostForSource, never an ffmpeg arg",
	}
)

// assertEveryFieldRoutedOrExempt / assertEncoderInputsAreRead are generic so a
// second fingerprint costs two call sites, not a second copy of the guard -
// duplicating a guard that exists to catch duplication drift would be its own
// joke. T is the fingerprint type; recv is how the mapping literals name it.
func assertEveryFieldRoutedOrExempt[T any](t *testing.T, typeName string, threadedMap, exemptMap map[string]string) {
	t.Helper()
	for f := range reflect.TypeFor[T]().Fields() {
		name := f.Name
		_, threaded := threadedMap[name]
		_, exempt := exemptMap[name]
		switch {
		case threaded && exempt:
			t.Errorf("%s.%s is declared both an encoder input and exempt - pick one", typeName, name)
		case !threaded && !exempt:
			t.Errorf("%s.%s is in neither map: declare where it reaches ffmpeg, "+
				"or why it stops at the fingerprint (see this file's header)", typeName, name)
		}
	}
}

// assertEncoderInputsAreRead proves each declared input is read INSIDE the opts
// literal that feeds ffmpeg - not merely mentioned somewhere in a file we named.
//
// It walks the package AST rather than grepping a hardcoded file list, which
// removes the two weaknesses of the first version: renaming or moving a mapping
// site no longer breaks the guard (nothing names files), and a field mentioned
// in a comment, a string or an unrelated function no longer counts as threaded.
// The read must appear within a composite literal of optsType - which is the
// only place that actually reaches the encoder.
func assertEncoderInputsAreRead(t *testing.T, typeName, optsType, recv string, threadedMap map[string]string) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()

	// Per-literal, not merged: the failure message can then say WHICH sites read
	// a field, which is what tells a reader whether an at-least-one pass is the
	// intended answer or an oversight.
	readBySite := map[string]map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isTypeNamed(lit.Type, optsType) {
				return true
			}
			site := fmt.Sprintf("%s:%d", name, fset.Position(lit.Pos()).Line)
			readBySite[site] = map[string]bool{}
			ast.Inspect(lit, func(in ast.Node) bool {
				// Receiver-bound: `fp.X` or `sess.fp.X`, never some other
				// struct that happens to carry a same-named field.
				if sel, ok := in.(*ast.SelectorExpr); ok && rootedAt(sel.X, recv) {
					readBySite[site][sel.Sel.Name] = true
				}
				return true
			})
			return true
		})
	}
	if len(readBySite) == 0 {
		t.Fatalf("no %s literal found in this package - did the mapping move to another package?", optsType)
	}

	for field := range threadedMap {
		var reading []string
		for site, fields := range readBySite {
			if fields[field] {
				reading = append(reading, site)
			}
		}
		if len(reading) == 0 {
			all := slices.Sorted(maps.Keys(readBySite))
			t.Errorf("%s.%s is declared an encoder input (%s) but no %s literal reads %s.%s - "+
				"the encoder never receives it, so the segments won't match what the hash promises. Sites: %s",
				typeName, field, threadedMap[field], optsType, recv, field, strings.Join(all, ", "))
		}
	}
}

// rootedAt reports whether e is `recv` or ends in `.recv` (e.g. `sess.fp`), so
// only reads OF THE FINGERPRINT count - a same-named field on another struct
// inside the same literal must not pass for threading.
func rootedAt(e ast.Expr, recv string) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == recv
	case *ast.SelectorExpr:
		return x.Sel.Name == recv
	}
	return false
}

// isTypeNamed matches both `Opts{...}` and `pkg.Opts{...}`.
func isTypeNamed(e ast.Expr, name string) bool {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name == name
	case *ast.SelectorExpr:
		return t.Sel.Name == name
	}
	return false
}

func TestDecisionFingerprint_EveryFieldIsRoutedOrExempt(t *testing.T) {
	assertEveryFieldRoutedOrExempt[DecisionFingerprint](t, "DecisionFingerprint",
		fingerprintEncoderInputs, fingerprintNonEncoderFields)
}

func TestDecisionFingerprint_EncoderInputsAreReadAtAMappingSite(t *testing.T) {
	assertEncoderInputsAreRead(t, "DecisionFingerprint", "HLSProcessOpts", "fp", fingerprintEncoderInputs)
}

// The AUDIO pipeline carries the same class of bug and was not mechanised: add
// a field to AudioFingerprint, hash it, forget to thread it into
// AudioProcessOpts, and the cache forks without the bytes changing (or the
// reverse). Correct today - this guard is here so it stays correct, since the
// video guard alone only proved the pattern was applied where it had already
// bitten.
var (
	audioFingerprintEncoderInputs = map[string]string{
		"SourcePath":       "AudioProcessOpts.FilePath",
		"SourceTrackIdx":   "AudioProcessOpts.SourceTrackIdx",
		"AudioCodec":       "AudioProcessOpts.AudioCodec",
		"AudioBitrate":     "AudioProcessOpts.AudioBitrate",
		"AudioNormalize":   "AudioProcessOpts.AudioNormalize",
		"MaxAudioChannels": "AudioProcessOpts.MaxAudioChannels",
		// Not an encode input in the codec sense, but it does reach ffmpeg (as
		// -output_ts_offset) and it changes the emitted bytes, so it belongs here
		// rather than in the exempt map.
		"VideoCompositionDelay": "AudioProcessOpts.VideoCompositionDelay",
	}
	audioFingerprintNonEncoderFields = map[string]string{
		"MediaID": "cache addressing only - two items sharing a source path must not share segments",
		"SourceDurationSec": "transport for the silence-tail bound - not an encode input, deliberately " +
			"NOT hashed (TestAudioFingerprint_SourceDurationSec_DoesNotForkHash pins that)",
	}
)

func TestAudioFingerprint_EveryFieldIsRoutedOrExempt(t *testing.T) {
	assertEveryFieldRoutedOrExempt[AudioFingerprint](t, "AudioFingerprint",
		audioFingerprintEncoderInputs, audioFingerprintNonEncoderFields)
}

func TestAudioFingerprint_EncoderInputsAreReadAtAMappingSite(t *testing.T) {
	assertEncoderInputsAreRead(t, "AudioFingerprint", "AudioProcessOpts", "fp", audioFingerprintEncoderInputs)
}
