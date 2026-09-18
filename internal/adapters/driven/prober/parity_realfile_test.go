package prober

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/langcode"
)

// TestParity_AgainstFFprobe maps every mkvgo-readable container in
// EVEY_PARITY_DIR (.mkv/.webm via the Matroska reader, .mp4/.m4v via mp4.OpenMeta
// - both routed by the production realMetaReader) AND via the ffprobe prober,
// then asserts the DECISION-AFFECTING fields agree. PATH-gated: skips unless
// EVEY_PARITY_DIR is set (no real-file paths are committed - run it locally,
// e.g. EVEY_PARITY_DIR=/media go test...). This is the real-file validation gate
// to pass before flipping the mp4 fast-path toggle on in prod (admin panel →
// Transcoding), the same way the mkv path was validated. Display-only fields
// (CodecLongName, ChannelLayout, per-stream
// Bitrate) are not compared; ffprobe attachment streams (which mkvgo does not
// surface as tracks) are ignored by comparing per stream-type.
func TestParity_AgainstFFprobe(t *testing.T) {
	dir := os.Getenv("EVEY_PARITY_DIR")
	if dir == "" {
		t.Skip("set EVEY_PARITY_DIR to a folder of real .mkv/.mp4 files to run the parity check")
	}
	var files []string
	// *.MOV uppercase matters: filepath.Glob is case-sensitive and real
	// iPhone/camera files ship as IMG_xxxx.MOV (production lowercases the ext,
	// so those files DO take the fast-path - the gate must see them too).
	for _, pat := range []string{"*.mkv", "*.webm", "*.mp4", "*.m4v", "*.m4a", "*.mov", "*.MOV"} {
		matched, _ := filepath.Glob(filepath.Join(dir, pat))
		files = append(files, matched...)
	}
	require.NotEmpty(t, files, "no .mkv/.webm/.mp4/.m4v/.mov in EVEY_PARITY_DIR")

	mr := realMetaReader{} // routes EBML vs ISO-BMFF exactly like the dispatcher
	ff := ffmpeg.NewProber("ffprobe")
	ctx := context.Background()
	var fastPath, fellBack int

	for _, f := range files {
		c, err := mr.ReadMeta(ctx, f)
		if err != nil {
			t.Logf("SKIP %s: mkvgo ReadMeta error %v", filepath.Base(f), err)
			continue
		}
		lower := strings.ToLower(f)
		ebml := strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".webm")
		mapped, ok := mapContainer(c, ebml)
		if !ok {
			fellBack++
			t.Logf("FALLBACK %s (mapper bailed -> ffprobe)", filepath.Base(f))
			continue
		}
		fastPath++

		ffRes, err := ff.ProbeStreams(ctx, f)
		require.NoError(t, err, f)

		name := filepath.Base(f)
		for _, st := range []library.StreamType{library.StreamTypeVideo, library.StreamTypeAudio, library.StreamTypeSubtitle} {
			mk := byType(mapped.Streams, st)
			fp := byType(ffRes.Streams, st)
			require.Lenf(t, mk, len(fp), "%s: %s stream count", name, st)
			for i := range mk {
				m, p := mk[i], fp[i]
				assert.Equalf(t, normCodec(p.Codec), normCodec(m.Codec), "%s %s[%d] codec", name, st, i)
				assert.Equalf(t, normLang(p.Language), normLang(m.Language), "%s %s[%d] language", name, st, i)
				assert.Equalf(t, p.IsDefault, m.IsDefault, "%s %s[%d] default", name, st, i)
				assert.Equalf(t, p.IsForced, m.IsForced, "%s %s[%d] forced", name, st, i)
				// CodecLongName/ChannelLayout are wired for consistency but NOT gated:
				// pure display strings whose exact ffmpeg wording ("5.1" vs "5.1(side)",
				// subtitle long names) is brittle and decision-irrelevant.
				if st == library.StreamTypeVideo {
					assert.Equalf(t, deref(p.Width), deref(m.Width), "%s video[%d] width", name, i)
					assert.Equalf(t, deref(p.Height), deref(m.Height), "%s video[%d] height", name, i)
					assert.Truef(t, colorSpaceEquiv(derefS(p.ColorSpace), derefS(m.ColorSpace), m.Codec),
						"%s video[%d] color_space: ffprobe=%q mkvgo=%q", name, i, derefS(p.ColorSpace), derefS(m.ColorSpace))
					assert.Equalf(t, deref(p.Level), deref(m.Level), "%s video[%d] level", name, i)
					assert.Equalf(t, fieldOrderNorm(derefS(p.FieldOrder)), fieldOrderNorm(derefS(m.FieldOrder)),
						"%s video[%d] field_order: ffprobe=%q mkvgo=%q", name, i, derefS(p.FieldOrder), derefS(m.FieldOrder))
					assert.Equalf(t, deref(p.DoViProfile), deref(m.DoViProfile), "%s video[%d] dovi_profile", name, i)
					assert.Equalf(t, deref(p.DoViBLCompat), deref(m.DoViBLCompat), "%s video[%d] dovi_bl_compat", name, i)
				}
				if st == library.StreamTypeAudio {
					assert.Equalf(t, deref(p.Channels), deref(m.Channels), "%s audio[%d] channels", name, i)
					assert.Truef(t, sampleRateEquiv(deref(p.SampleRate), deref(m.SampleRate)),
						"%s audio[%d] sample_rate: ffprobe=%d mkvgo=%d", name, i, deref(p.SampleRate), deref(m.SampleRate))
				}
			}
		}
	}
	t.Logf("parity: %d fast-path, %d fell back to ffprobe (of %d files)", fastPath, fellBack, len(files))
	// Every assertion above lives inside the fast-path branch. If mkvgo errored
	// or the mapper bailed on ALL of them, the loop `continue`s throughout and
	// the test reports GREEN having compared nothing - the exact shape of gate
	// that cannot be seen going red. A run that produced zero fast-path files
	// is a broken fixture (or a broken reader), never a pass.
	require.Positivef(t, fastPath, "parity proved nothing: 0 of %d files took the mkvgo fast path (%d fell back)", len(files), fellBack)
}

// normCodec folds container-specific spellings of the same codec so the MP4
// (mkvgo) and ffprobe vocabularies compare equal: ffprobe reports the MP4
// timed-text track as "mov_text" while mkvgo (like the decision's text-class
// handling) treats it as "subrip" - same text-subtitle class either way.
func normCodec(c string) string {
	if c == "mov_text" {
		return "subrip"
	}
	return c
}

// colorSpaceEquiv accepts an exact match, or mkvgo "" on an h264 stream: h264
// colour can live in an in-band SPS the head-only reader cannot see (a documented
// limit), and "" is SDR-safe (never a false HDR).
func colorSpaceEquiv(ff, mk, codec string) bool {
	return ff == mk || (mk == "" && codec == "h264")
}

// fieldOrderNorm collapses both vocabularies to the decision-relevant bit
// through the domain reader: "is it interlaced", not the exact field order.
func fieldOrderNorm(s string) string { return string(library.ScanTypeOf(s)) }

// sampleRateEquiv accepts an exact match or the implicit-SBR 2× case (mkvgo reads
// the coded core rate head-only; ffprobe decodes the SBR-doubled output).
func sampleRateEquiv(ff, mk int) bool {
	return ff == mk || (mk > 0 && ff == mk*2)
}

func byType(s []*library.MediaStream, t library.StreamType) []*library.MediaStream {
	var out []*library.MediaStream
	for _, x := range s {
		if x.StreamType == t {
			out = append(out, x)
		}
	}
	return out
}
func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
func derefS(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// normLang folds the ffprobe ISO-639-2 vocabulary (fre/ger/tur/dut/...) and
// mkvgo's BCP-47 (fr/de/tr/nl/...) to a common 2-letter form, using the SAME
// shared table the production audio_track_resolver uses (pkg/langcode) so the
// parity gate can't drift from real behaviour. "und"/"" are treated as equal.
func normLang(s string) string {
	n := langcode.Normalize(s)
	if n == "und" {
		return ""
	}
	return n
}
