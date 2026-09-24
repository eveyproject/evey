package transcoding

import (
	"reflect"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The DecisionFingerprint is three things in one struct (ADR-0030, 5): the
// cache IDENTITY (what Hash serialises - different value, different bytes,
// different cache entry), the RUNTIME options the builder receives but that
// do not change the addressable bytes (excluded from Hash on purpose: a GPU-
// vs CPU-tone-mapped segment is interchangeable), and the ADMISSION inputs
// (priced, never encoded). This guard forces every field into exactly one
// bucket and PROVES the bucket by mutation: an identity field must move the
// hash, a runtime or admission field must not. A new field lands here before
// it can silently fork the cache or silently alias two byte-streams.
var (
	fingerprintIdentityFields = map[string]string{
		"MediaID":             "media=",
		"SourcePath":          "src=",
		"VideoCodec":          "vcodec=",
		"AudioCodec":          "acodec=",
		"AudioEncoder":        "aenc= (non-native only)",
		"AudioTrack":          "atrack=",
		"SubtitleBurnInTrack": "sub=",
		"HDRTonemap":          "hdr= and tm=",
		"AudioNormalize":      "anorm=",
		"MaxAudioChannels":    "ach= and dmx=",
		"SourceAudioChannels": "dmx= (the fold policy the channel count selects)",
		"ForceSidecarSubs":    "sidecar=",
		"UseFMP4":             "fmp4=",
		"Grid":                "grid= (a frame-exact grid only: the legacy clock hashes as nothing)",
		"HWAccel":             "hwenc= (an encode session only: two encoders, two inits)",
		"AudioCopy":           "acopy= and the absence of dmx=",
		"AudioOffsetMs":       "aoffset= (non-zero only)",
		"DeinterlaceAll":      "deintall= (flagged only)",
		"Telecine":            "ivtc= (flagged only)",
		"HDRPassthrough":      "hdrpass=",
		"HWTonemap":           "tm= (the tone-map implementation draws a different picture)",
		"HWFeatures":          "tm= (OpenCL / Vulkan / CUDA / VPP curves differ) - excluded otherwise",
		"SourceSAR":           "sar= (anamorphic only)",
		"MaxH264Level":        "h264lvl= (when a rung is capped by it)",
		"Profiles":            "p= per rung",
	}
	// Encoder inputs that do not change the addressable bytes: excluded from
	// Hash, each with the reason. What RuntimeOptions would carry.
	fingerprintRuntimeFields = map[string]string{
		"HWDevice":            "which render node runs the same encoder",
		"HWSoftwareDecode":    "the decode side of the same picture",
		"HWOverlay":           "who blends the same subtitle onto the same frames",
		"HWDeinterlace":       "which deinterlacer produces the same progressive frames",
		"HDRTransfer":         "derived from the source, restated for the builder",
		"SubtitleBurnInCodec": "derived from the source track the index already names",
		"SourceVideoCodec":    "derived from MediaID (the copy path's -tag:v)",
		"QualityTargets":      "excluded while the only policy is the static one (see the field)",
	}
	// Priced by admission, never encoded, never hashed.
	fingerprintAdmissionFields = map[string]string{
		"SourceVideoHeight": "decode workload",
		"SourceFPS":         "required rate",
	}
)

// identityBaseline is a fingerprint in which every identity field is in the
// state where a mutation moves the hash (a transcode rung so hwenc= is emitted,
// a tone-map so tm= is, a fold so dmx= is, a level cap that bites...).
func identityBaseline() DecisionFingerprint {
	return DecisionFingerprint{
		MediaID: "m", SourcePath: "/s.mkv", VideoCodec: "h264", AudioCodec: "aac",
		AudioTrack: 0, SubtitleBurnInTrack: -1,
		HDRTonemap: true, HWTonemap: true, HWFeatures: []string{transcoding.FeatureTonemapVulkan},
		MaxAudioChannels: 2, SourceAudioChannels: 6,
		UseFMP4: true, HWAccel: "vaapi", HWDevice: "/dev/dri/renderD128",
		MaxH264Level: 41, HDRTransfer: "pq", SourceVideoCodec: "hevc", SourceVideoHeight: 2160, SourceFPS: 24,
		Profiles: []ProfileFingerprint{{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 20000, AudioBitrate: 192}},
	}
}

// mutate returns a copy of fp with the named field changed to another legal
// value. The generic rule (flip / +1 / append) covers most fields; the ones
// whose hash contribution is a POLICY of the value get a targeted change.
func mutate(t *testing.T, fp DecisionFingerprint, field string) DecisionFingerprint {
	t.Helper()
	switch field {
	case "SourceAudioChannels":
		fp.SourceAudioChannels = 2 // 6 -> 2: no fold left to apply
		return fp
	case "HWFeatures":
		fp.HWFeatures = []string{transcoding.FeatureTonemapOpenCL} // another curve
		return fp
	case "SourceSAR":
		fp.SourceSAR = "4:3"
		return fp
	case "AudioEncoder":
		fp.AudioEncoder = "libfdk_aac"
		return fp
	case "Profiles":
		fp.Profiles = append(fp.Profiles, ProfileFingerprint{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 192})
		return fp
	case "QualityTargets":
		fp.QualityTargets = []transcoding.QualityTarget{{Mode: transcoding.RateControlConstantQuality}}
		return fp
	case "Grid":
		fp.Grid = transcoding.GridForFrameRate(23.976)
		return fp
	}
	v := reflect.ValueOf(&fp).Elem().FieldByName(field)
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Int, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 1)
	case reflect.Float64:
		v.SetFloat(v.Float() + 1)
	case reflect.String:
		v.SetString(v.String() + "x")
	case reflect.Slice:
		v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
	default:
		t.Fatalf("no mutation rule for %s (%s)", field, v.Kind())
	}
	return fp
}

func TestDecisionFingerprint_EveryFieldIsIdentityRuntimeOrAdmission(t *testing.T) {
	base := identityBaseline()
	baseHash := base.Hash()
	for f := range reflect.TypeFor[DecisionFingerprint]().Fields() {
		name := f.Name
		_, id := fingerprintIdentityFields[name]
		_, rt := fingerprintRuntimeFields[name]
		_, adm := fingerprintAdmissionFields[name]
		n := 0
		for _, b := range []bool{id, rt, adm} {
			if b {
				n++
			}
		}
		if n != 1 {
			t.Errorf("DecisionFingerprint.%s is in %d buckets: declare it identity, runtime or admission (see this file's header)", name, n)
			continue
		}
		moved := mutate(t, base, name).Hash() != baseHash
		switch {
		case id && !moved:
			t.Errorf("DecisionFingerprint.%s is declared identity (%s) but changing it left the hash alone", name, fingerprintIdentityFields[name])
		case !id && moved:
			t.Errorf("DecisionFingerprint.%s is declared %s but changing it moved the hash - it forks the cache", name, map[bool]string{rt: "runtime", adm: "admission"}[true])
		}
	}
}
