package prober

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/domain/library"
)

// TestParity_AgainstFFprobe maps every mkvgo-readable container in
// EVEY_PARITY_DIR (.mkv/.webm via the Matroska reader, .mp4/.m4v via mp4.OpenMeta
// — both routed by the production realMetaReader) AND via the ffprobe prober,
// then asserts the DECISION-AFFECTING fields agree. PATH-gated: skips unless
// EVEY_PARITY_DIR is set (no real-file paths are committed — run it locally,
// e.g. EVEY_PARITY_DIR=/media go test...). This is the real-file validation gate
// to pass before enabling EVEY_MP4_FASTPATH in prod, the same way the mkv path
// was validated. Display-only fields (CodecLongName, ChannelLayout, per-stream
// Bitrate) are not compared; ffprobe attachment streams (which mkvgo does not
// surface as tracks) are ignored by comparing per stream-type.
func TestParity_AgainstFFprobe(t *testing.T) {
	dir := os.Getenv("EVEY_PARITY_DIR")
	if dir == "" {
		t.Skip("set EVEY_PARITY_DIR to a folder of real .mkv/.mp4 files to run the parity check")
	}
	var files []string
	for _, pat := range []string{"*.mkv", "*.webm", "*.mp4", "*.m4v"} {
		matched, _ := filepath.Glob(filepath.Join(dir, pat))
		files = append(files, matched...)
	}
	require.NotEmpty(t, files, "no .mkv/.webm/.mp4/.m4v in EVEY_PARITY_DIR")

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
		mapped, ok := mapContainer(c)
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
				assert.Equalf(t, p.Codec, m.Codec, "%s %s[%d] codec", name, st, i)
				assert.Equalf(t, normLang(p.Language), normLang(m.Language), "%s %s[%d] language", name, st, i)
				assert.Equalf(t, p.IsDefault, m.IsDefault, "%s %s[%d] default", name, st, i)
				assert.Equalf(t, p.IsForced, m.IsForced, "%s %s[%d] forced", name, st, i)
				if st == library.StreamTypeVideo {
					assert.Equalf(t, deref(p.Width), deref(m.Width), "%s video[%d] width", name, i)
					assert.Equalf(t, deref(p.Height), deref(m.Height), "%s video[%d] height", name, i)
					assert.Equalf(t, derefS(p.ColorSpace), derefS(m.ColorSpace), "%s video[%d] color_space", name, i)
				}
				if st == library.StreamTypeAudio {
					assert.Equalf(t, deref(p.Channels), deref(m.Channels), "%s audio[%d] channels", name, i)
				}
			}
		}
	}
	t.Logf("parity: %d fast-path, %d fell back to ffprobe (of %d files)", fastPath, fellBack, len(files))
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

// normLang folds the ffprobe ISO-639-2 vocabulary (fre/ger/...) and mkvgo's
// BCP-47 (fr/de/...) to a common 2-letter form for comparison, matching how the
// decision's audio_track_resolver normalizes before use.
func normLang(s string) string {
	switch s {
	case "fre", "fra":
		return "fr"
	case "ger", "deu":
		return "de"
	case "eng":
		return "en"
	case "spa":
		return "es"
	case "ita":
		return "it"
	case "chi", "zho", "cmn", "yue", "wuu", "hak", "nan":
		return "zh" // legacy 639-2 (ffprobe) and BCP-47 macrolang (mkvgo) both -> zh
	case "und", "":
		return ""
	}
	if len(s) > 2 {
		return s[:2]
	}
	return s
}
