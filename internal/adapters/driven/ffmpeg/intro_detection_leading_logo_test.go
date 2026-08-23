package ffmpeg

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blackdetectLine renders one ffmpeg blackdetect line the way parseLeadingLogoBlock
// reads it: offsets relative to the decode start (t=0, since the scan always
// begins at -ss 0).
func blackdetectLine(startSec, endSec float64) string {
	return fmt.Sprintf("[blackdetect @ 0x1] black_start:%.3f black_end:%.3f black_duration:%.3f",
		startSec, endSec, endSec-startSec)
}

// A real distributor-logo block: three short black holds within the first
// 30s, gaps well inside leadingLogoClusterGapSec.
func TestParseLeadingLogoBlock_ThreeRegionClusterAccepted(t *testing.T) {
	t.Parallel()
	output := blackdetectLine(0, 1.0) + "\n" +
		blackdetectLine(8, 9.5) + "\n" +
		blackdetectLine(20, 22.0)
	startMs, endMs, found := parseLeadingLogoBlock(output, 240, 6000)
	require.True(t, found)
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(22_000), endMs)
}

// A film that opens straight on picture (no black near t=0) has no logo
// block to find - the first black region sits at 40s, well past
// leadingLogoFirstRegionMaxStartSec.
func TestParseLeadingLogoBlock_NoOpeningBlackRefused(t *testing.T) {
	t.Parallel()
	output := blackdetectLine(40, 42.0)
	_, _, found := parseLeadingLogoBlock(output, 240, 6000)
	assert.False(t, found, "a first black region at 40s is a scene fade, not an opening logo block")
}

// A single, short opening fade (a scene fade-in, not a held distributor
// card) must be refused: it's the one-region case that isn't trusted unless
// it carries enough black.
func TestParseLeadingLogoBlock_ShortIsolatedFadeRefused(t *testing.T) {
	t.Parallel()
	output := blackdetectLine(0, 1.2)
	_, _, found := parseLeadingLogoBlock(output, 240, 6000)
	assert.False(t, found, "a lone brief fade-in is a scene opening, not a logo block")
}

// A single region CAN be trusted alone when it carries a long held black
// (a slow fade-in behind a distributor card), as long as it also clears the
// 12s service floor.
func TestParseLeadingLogoBlock_SingleLongRegionAcceptedAboveFloor(t *testing.T) {
	t.Parallel()
	output := blackdetectLine(0, 15.0)
	startMs, endMs, found := parseLeadingLogoBlock(output, 240, 6000)
	require.True(t, found)
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(15_000), endMs)
}

// A plausible, trusted cluster that still ends below the 12s service floor
// (minDetectedIntroMs) must be refused: it would never be served, so it's
// not worth storing.
func TestParseLeadingLogoBlock_BelowServiceFloorRefused(t *testing.T) {
	t.Parallel()
	output := blackdetectLine(0, 0.6) + "\n" + blackdetectLine(3, 3.6)
	startMs, endMs, found := parseLeadingLogoBlock(output, 240, 6000)
	assert.False(t, found, "cluster ends at 3.6s, below the 12s serving floor")
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(0), endMs)
}

// An oversized block (past the absolute plausibility cap) is refused even
// though every individual region looks like a real black. Chained regions
// 25s apart (gap 20s, within leadingLogoClusterGapSec) keep the cluster
// growing well past the 300s cap; durationSec is set very high so the
// relative-to-runtime cap does not fire instead - this isolates the
// absolute cap.
func TestParseLeadingLogoBlock_OversizedBlockRefused(t *testing.T) {
	t.Parallel()
	var lines []string
	for i := range 15 {
		start := float64(i) * 25
		lines = append(lines, blackdetectLine(start, start+5))
	}
	output := strings.Join(lines, "\n")
	_, _, found := parseLeadingLogoBlock(output, 400, 20_000)
	assert.False(t, found, "a 355s 'logo block' exceeds the absolute plausibility cap")
}

// A block plausible in absolute seconds but too large relative to a short
// film's own runtime is refused too.
func TestParseLeadingLogoBlock_OversizedRelativeToDurationRefused(t *testing.T) {
	t.Parallel()
	// 150s block on a 20-minute (1200s) film: 12.5% of runtime, above the
	// leadingLogoMaxDurationFraction cap.
	output := blackdetectLine(0, 5.0) + "\n" + blackdetectLine(20, 150.0)
	_, _, found := parseLeadingLogoBlock(output, 240, 1200)
	assert.False(t, found, "150s of 'logos' on a 20-minute film is implausible relative to runtime")
}

// Empty blackdetect output (no black at all near the head) is refused.
func TestParseLeadingLogoBlock_EmptyOutputRefused(t *testing.T) {
	t.Parallel()
	startMs, endMs, found := parseLeadingLogoBlock("", 240, 6000)
	assert.False(t, found)
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(0), endMs)
}
