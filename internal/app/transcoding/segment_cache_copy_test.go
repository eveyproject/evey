package transcoding

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// The fingerprint half of the cache invariant: Hash() MUST derive from every
// byte-affecting dimension of the decision. Copy was forgotten once (copy and
// transcode segments at the same WxH aliased in the cache = corrupt stream);
// these tests close that class - the first concretely, the second structurally.

func TestDecisionFingerprint_CopyVsTranscodeDiffer(t *testing.T) {
	transcodeFP := DecisionFingerprint{
		MediaID:    "m1",
		SourcePath: "/media/x.mkv",
		VideoCodec: "h264",
		AudioCodec: "aac",
		Profiles: []ProfileFingerprint{
			{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192, Copy: false},
		},
	}
	copyFP := transcodeFP
	copyFP.Profiles = []ProfileFingerprint{
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192, Copy: true},
	}
	assert.NotEqual(t, transcodeFP.Hash(), copyFP.Hash(),
		"a copy rung and a transcode rung at the same WxH produce different bytes - they MUST hash differently")
}

// TestProfileFingerprint_EveryFieldAffectsHash mechanically enforces that EVERY
// field of ProfileFingerprint is folded into DecisionFingerprint.Hash(). Add a
// byte-affecting field and forget the Hash() line → this fails. The structural
// guard against the next Copy-style omission (the user's "correct-by-construction,
// not a manual list" point made executable).
func TestProfileFingerprint_EveryFieldAffectsHash(t *testing.T) {
	mk := func() DecisionFingerprint {
		return DecisionFingerprint{
			MediaID: "m1", SourcePath: "/media/x.mkv", VideoCodec: "h264", AudioCodec: "aac",
			Profiles: []ProfileFingerprint{
				{Name: "x", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 128},
			},
		}
	}
	baseHash := mk().Hash()
	pt := reflect.TypeFor[ProfileFingerprint]()
	for i := range pt.NumField() {
		fp := mk()
		f := reflect.ValueOf(&fp.Profiles[0]).Elem().Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(f.String() + "_mut")
		case reflect.Int, reflect.Int64:
			f.SetInt(f.Int() + 1)
		case reflect.Bool:
			f.SetBool(!f.Bool())
		default:
			t.Fatalf("unhandled ProfileFingerprint field kind %s (%s) - extend this test", f.Kind(), pt.Field(i).Name)
		}
		if fp.Hash() == baseHash {
			t.Errorf("ProfileFingerprint.%s does NOT affect Hash() - a byte-affecting dimension is omitted (the Copy-aliasing class)", pt.Field(i).Name)
		}
	}
}

// TestProfileFingerprintOf_CopiesEveryField is the SECOND reflection link: it
// guards the HLSProfile → ProfileFingerprint projection. Both the source values
// and the field comparison are reflection-built (no manual list on either side),
// so a ProfileFingerprint field not copied from its same-named HLSProfile field
// reddens - closing the omission class end-to-end with the Hash() guard above.
func TestProfileFingerprintOf_CopiesEveryField(t *testing.T) {
	// Set EVERY HLSProfile field to a distinct non-zero value via reflection so a
	// future field is auto-covered; a forgotten copy leaves the zero value and is
	// then detectable.
	src := transcoding.HLSProfile{}
	sv := reflect.ValueOf(&src).Elem()
	for i := range sv.NumField() {
		f := sv.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("v" + strconv.Itoa(i))
		case reflect.Int, reflect.Int64:
			f.SetInt(int64(i) + 1)
		case reflect.Bool:
			f.SetBool(true)
		default:
			t.Fatalf("unhandled HLSProfile field kind %s (%s) - extend this test", f.Kind(), sv.Type().Field(i).Name)
		}
	}

	got := ProfileFingerprintOf(src)
	gv := reflect.ValueOf(got)
	gt := gv.Type()
	for i := range gt.NumField() {
		name := gt.Field(i).Name
		srcF := sv.FieldByName(name)
		if !srcF.IsValid() {
			t.Fatalf("ProfileFingerprint.%s has no same-named HLSProfile field - reconcile the structs or carry it explicitly in ProfileFingerprintOf", name)
		}
		if !reflect.DeepEqual(srcF.Interface(), gv.Field(i).Interface()) {
			t.Errorf("ProfileFingerprintOf did NOT copy %s (HLSProfile=%v, ProfileFingerprint=%v) - byte-affecting projection omitted",
				name, srcF.Interface(), gv.Field(i).Interface())
		}
	}
}
