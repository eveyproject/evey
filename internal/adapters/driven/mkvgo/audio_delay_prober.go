package mkvgo

// Native audio start-delay diagnosis for the seek-index scan: the maximum,
// across a file's audio tracks, of how late the audio starts relative to the
// video - the "repack mux defect" the scan flags. It replaces the ffprobe
// prober (one subprocess per file, every scan) with mkvgo's head + first-
// clusters read. Matroska/WebM only, which is exactly the seek-index scan's
// candidate set.

import (
	"context"
	"fmt"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"
)

// AudioDelayProber implements the maintenance.AudioDelayProber port over
// mkvgo's native AudioStartDelays - no ffprobe.
type AudioDelayProber struct{}

// NewAudioDelayProber returns the native prober.
func NewAudioDelayProber() *AudioDelayProber { return &AudioDelayProber{} }

// ProbeMaxAudioStartDelaySec returns the largest audio start delay across the
// file's tracks, in seconds (0 when aligned or not Matroska). A non-Matroska
// file has no EBML tracks to read and reports 0 - the scan only feeds EBML
// candidates here anyway.
func (AudioDelayProber) ProbeMaxAudioStartDelaySec(ctx context.Context, path string) (float64, error) {
	if !IsEBMLContainer(path) {
		return 0, nil
	}
	delays, err := extmatroska.AudioStartDelays(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("audio-delay probe: %w", err)
	}
	var maxNs int64
	for _, ns := range delays {
		if ns > maxNs {
			maxNs = ns
		}
	}
	return float64(maxNs) / 1e9, nil
}
