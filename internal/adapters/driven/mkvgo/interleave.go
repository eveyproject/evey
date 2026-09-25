package mkvgo

import (
	"context"
	"fmt"
	"io"
	"os"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

// interleaveSamples offsets per file: ~11 MB read. Sparser sampling can land
// on a mild boundary (17 s where 32 samples read 532 s on the same file).
// Blind to a THIN block, smaller than the step between samples (a stereo
// track's 180 s blocks read 0, measured 2026-09-25); the exact measure (mkvgo
// Analyze, every cluster) reads the whole file and was judged too costly for
// the NAS.
const interleaveSamples = 16

// interleaveScanCap bounds the scan to the next Cluster from a sampled offset
// (muxers close clusters every ~5 s / ~5 MB; bigger ones just yield no sample).
const interleaveScanCap = int64(8 << 20)

// ProbeInterleaveBackstepSec implements maintenance.InterleaveProber. A muxer
// fed by two demuxers of one file (ffmpeg -itsoffset remux) writes minutes of
// video then minutes of audio: cluster timestamps fall back at every block
// boundary where a sane file's only rise. 0 = no finding (well-interleaved,
// non-EBML, or fewer than two samples found a cluster).
func (a *Adapter) ProbeInterleaveBackstepSec(ctx context.Context, path string) (float64, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ProbeInterleaveBackstep")
	defer span.End()

	if !IsEBMLContainer(path) {
		return 0, nil
	}
	c, err := reader.OpenMeta(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("interleave probe: open: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("interleave probe: %w", err)
	}
	defer f.Close()
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("interleave probe: %w", err)
	}
	scale := c.Info.TimecodeScale
	if scale == 0 {
		scale = extmkv.DefaultTimecodeScale
	}
	stamps, err := sampleClusterTimestamps(ctx, f, c.SegmentStart, size, scale)
	if err != nil {
		return 0, fmt.Errorf("interleave probe: %w", err)
	}
	return float64(maxBackstepMs(stamps)) / 1000, nil
}

// sampleClusterTimestamps returns, in file order, the timestamp (ms) of the
// first valid Cluster at or after each sampled offset of [segStart, size).
func sampleClusterTimestamps(ctx context.Context, f io.ReadSeeker, segStart, size, scale int64) ([]int64, error) {
	body := size - segStart
	if body <= 0 {
		return nil, nil
	}
	var out []int64
	for i := range interleaveSamples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		off := segStart + body*int64(i)/interleaveSamples
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			return nil, err
		}
		limit := min(off+interleaveScanCap, size)
		clusterOff, err := reader.ResyncToCluster(f, limit)
		if err != nil {
			return nil, err
		}
		if clusterOff < 0 {
			continue
		}
		br, err := reader.NewBlockReaderAt(f, scale, clusterOff)
		if err != nil {
			return nil, err
		}
		br.SetHeaderOnly(true)
		if _, err := br.Next(); err != nil {
			continue // a cluster with no readable block is no sample
		}
		out = append(out, br.Pos().ClusterTS*scale/1_000_000)
	}
	return out, nil
}

// maxBackstepMs is the largest drop between consecutive timestamps (0 when
// they never decrease).
func maxBackstepMs(stamps []int64) int64 {
	var worst int64
	for i := 1; i < len(stamps); i++ {
		if d := stamps[i-1] - stamps[i]; d > worst {
			worst = d
		}
	}
	return worst
}

// ProbeMuxingApp implements maintenance.MuxerProber: the Segment Info's
// MuxingApp (head-only), "" for a non-EBML container.
func (a *Adapter) ProbeMuxingApp(ctx context.Context, path string) (string, error) {
	if !IsEBMLContainer(path) {
		return "", nil
	}
	c, err := reader.OpenMeta(ctx, path)
	if err != nil {
		return "", fmt.Errorf("muxer probe: %w", err)
	}
	return c.Info.MuxingApp, nil
}
