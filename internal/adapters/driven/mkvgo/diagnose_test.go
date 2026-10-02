package mkvgo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
)

// The truncated verdict must come from mkvgo's CLASSIFIED finding, not the raw
// salvage-walk TruncatedTail bit. Proven wrong on prod bytes (2026-07-26,
// "Scary movie 1"): declared end 9 bytes SHORT of the file size (trailing
// junk) + mid-file damage - the walk's tail hit EOF inside the junk, so
// TruncatedTail was true and the adapter mislabeled a complete, playable file
// "incomplete download / re-acquire the source".
func TestMapDiagnosis_TruncatedFollowsTheFinding(t *testing.T) {
	// Scary-movie-1 shape: damaged + trailing-junk findings, NO truncated one.
	junky := mapDiagnosis(&extmatroska.Diagnosis{
		Findings: []extmatroska.Finding{
			{Kind: "damaged", Detail: "1 damaged range(s)"},
			{Kind: "trailing-junk", Detail: "9 byte(s) beyond the declared Segment end"},
		},
		Damage: &extmatroska.SalvageReport{TruncatedTail: true, BytesSkipped: 268405},
	})
	assert.False(t, junky.Truncated, "no truncated finding -> not an incomplete download")
	assert.True(t, junky.TrailingJunk())
	assert.EqualValues(t, 268405, junky.DamagedBytes, "the mid-file damage stays resync-repairable")

	// A genuine incomplete download carries the classified finding.
	cut := mapDiagnosis(&extmatroska.Diagnosis{
		Findings: []extmatroska.Finding{
			{Kind: "truncated", Detail: "the source ends early"},
		},
		Damage: &extmatroska.SalvageReport{TruncatedTail: true},
	})
	assert.True(t, cut.Truncated)
}

// A complete MKV with surplus bytes appended PAST its declared Segment end is
// trailing junk, not damage: mkvgo v0.25 raises the `trailing-junk` finding
// (and no longer the truncated verdict) for bytes lying entirely beyond the
// Segment, but those bytes still inflate the salvage walk's BytesSkipped. The
// adapter must surface TrailingJunk() true, keep Truncated false, and NOT relay
// the surplus as DamagedBytes - otherwise a whole, playable file is shown as
// corrupt and the operator is told to re-download or resync it for nothing.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run TestDiagnoseTrailingJunk -v
func TestDiagnoseTrailingJunkIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1")
	}
	ctx := t.Context()
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.mkv")
	// A SEEKABLE output (ffmpeg's own file argument, not a pipe): the muxer
	// seeks back to seal the real Segment size and writes a Cues element. That
	// known declared end is what makes "declared < actual" - trailing junk -
	// detectable; a piped, unknown-size Segment could never be.
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=300", "-t", "6",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48",
		"-c:a", "aac", clean)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}

	a := New()

	// Baseline: the pristine file is not flagged for any tail defect.
	base, err := a.Diagnose(ctx, clean)
	require.NoError(t, err)
	require.False(t, base.TrailingJunk(), "a clean file must not report trailing junk")
	require.False(t, base.Truncated, "a clean file must not report truncation")
	require.Zero(t, base.DamagedBytes, "a clean file has no damaged bytes")

	// Append 8 KiB of junk PAST the declared Segment end.
	junky := filepath.Join(dir, "junky.mkv")
	data, err := os.ReadFile(clean)
	require.NoError(t, err)
	junk := make([]byte, 8192)
	for i := range junk {
		junk[i] = byte(i*31 + 7)
	}
	require.NoError(t, os.WriteFile(junky, append(append([]byte{}, data...), junk...), 0o644))

	d, err := a.Diagnose(ctx, junky)
	require.NoError(t, err)
	assert.True(t, d.TrailingJunk(), "surplus bytes past the Segment end must raise trailing-junk")
	assert.False(t, d.Truncated, "trailing junk is complete media, never a truncation")
	assert.False(t, d.Damaged(), "trailing junk is not mid-Segment damage")
	assert.Zero(t, d.DamagedBytes, "the surplus tail must NOT be relayed as damaged bytes")
}

// The audio tail and the missing-tail magnitude are mkvgo's measurements now
// (TrackEnds, Diagnosis.MissingTailBytes): the adapter relays them, it does
// not re-derive them from the container header or a hand-rolled tail walk.
func TestMapDiagnosis_RelaysTrackEndsAndMissingTail(t *testing.T) {
	short := mapDiagnosis(&extmatroska.Diagnosis{
		Healthy:  false,
		Findings: []extmatroska.Finding{{Kind: "audio-short", Track: 2}},
		TrackEnds: &extmatroska.TrackEndsReport{
			DeclaredDurationMs: 5_400_000,
			VideoEndMs:         5_398_000,
			AudioShortfallMs:   249_600,
			ShortAudioTrack:    2,
		},
	})
	if short.AudioShortfallMs != 249_600 {
		t.Fatalf("AudioShortfallMs = %d, want 249600", short.AudioShortfallMs)
	}

	clean := mapDiagnosis(&extmatroska.Diagnosis{Healthy: true, TrackEnds: &extmatroska.TrackEndsReport{VideoEndMs: 100}})
	if clean.AudioShortfallMs != 0 {
		t.Fatalf("no shortfall must map to 0, got %d", clean.AudioShortfallMs)
	}

	cut := mapDiagnosis(&extmatroska.Diagnosis{
		Findings:         []extmatroska.Finding{{Kind: "truncated"}},
		FileSize:         900,
		DeclaredSize:     41_000_900,
		MissingTailBytes: 41_000_000,
	})
	if !cut.Truncated || cut.MissingTailBytes != 41_000_000 {
		t.Fatalf("truncated=%v missing=%d, want true/41000000", cut.Truncated, cut.MissingTailBytes)
	}

	// A missing-tail number without the classified finding is trailing-junk
	// noise from the raw walk, never a cut: it must not leak through.
	junky := mapDiagnosis(&extmatroska.Diagnosis{
		Findings:         []extmatroska.Finding{{Kind: "trailing-junk"}},
		MissingTailBytes: 9,
	})
	if junky.Truncated || junky.MissingTailBytes != 0 {
		t.Fatalf("junk must not read as a cut: truncated=%v missing=%d", junky.Truncated, junky.MissingTailBytes)
	}
}

// Since mkvgo v0.36 a file cut at the end AND holed in the middle (four real
// "9 1 1" episodes) carries both findings. It is relayed as both - the holes
// stay measured - and the maintenance routes it as an incomplete download
// (SeekStatus.Verdict ranks Truncated first), which is what mkvgo advises too.
func TestMapDiagnosis_TruncatedAndDamaged(t *testing.T) {
	d := mapDiagnosis(&extmatroska.Diagnosis{
		Findings: []extmatroska.Finding{
			{Kind: "no-index", Detail: "no Cues"},
			{Kind: "truncated", Detail: "the source ends early"},
			{Kind: "damaged", Detail: "2 damaged range(s)", Remedy: "re-download the source (mkvgo reindex --resync keeps what the file still holds meanwhile)"},
		},
		MissingTailBytes: 4096,
		Damage:           &extmatroska.SalvageReport{TruncatedTail: true, BytesSkipped: 6_291_456},
	})
	assert.True(t, d.Truncated)
	assert.True(t, d.Damaged())
	assert.EqualValues(t, 4096, d.MissingTailBytes)
	assert.EqualValues(t, 6_291_456, d.DamagedBytes)
}
