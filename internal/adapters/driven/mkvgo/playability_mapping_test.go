package mkvgo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gravity-zero/mkvgo/mkv/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/mkv"
)

// The app-owned target crosses into mkvgo field by field: a swapped width
// and height, or a dropped level cap, would silently change every verdict.
func TestToOpsTarget_MapsEveryField(t *testing.T) {
	got := toOpsTarget(apptranscoding.PlayabilityTarget{
		Name: "chrome", Containers: []string{"mp4", "hls"}, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac", "opus"},
		MaxWidth: 3840, MaxHeight: 2160, HDR: true, MaxLevelH264: 51, HEVCMain10: true, VP9Profile2: true,
	})
	assert.Equal(t, ops.Target{
		Name: "chrome", Container: []string{"mp4", "hls"}, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac", "opus"},
		MaxWidth: 3840, MaxHeight: 2160, HDR: true, MaxLevelH264: 51, HEVCMain10: true, VP9Profile2: true,
	}, got)
	assert.Equal(t, "custom", toOpsTarget(apptranscoding.PlayabilityTarget{}).Name, "an unnamed target is mkvgo's custom profile")
	assert.Zero(t, got.MaxLevelHEVC, "no source for the HEVC cap: mkvgo's strictest default")
	assert.False(t, got.DolbyVision)
}

func TestFromOpsPlayabilityReport_KeepsTrackOrderAndReasons(t *testing.T) {
	got := fromOpsPlayabilityReport(&ops.PlayabilityReport{
		Target: "chrome", OverallVerdict: "remux", RemuxContainer: "mp4",
		Tracks: []ops.TrackVerdict{
			{TrackID: 1, Type: "video", Verdict: "direct-play"},
			{TrackID: 2, Type: "audio", Verdict: "transcode", Reasons: []string{"codec dts not in target"}},
		},
	})
	assert.Equal(t, "chrome", got.Target)
	assert.Equal(t, "remux", got.OverallVerdict)
	assert.Equal(t, "mp4", got.RemuxContainer)
	assert.Equal(t, []apptranscoding.PlayabilityTrackVerdict{
		{TrackID: 1, Type: "video", Verdict: "direct-play"},
		{TrackID: 2, Type: "audio", Verdict: "transcode", Reasons: []string{"codec dts not in target"}},
	}, got.Tracks)
	assert.Empty(t, fromOpsPlayabilityReport(&ops.PlayabilityReport{}).Tracks)
}

func TestFmtBytes(t *testing.T) {
	for n, want := range map[uint64]string{
		0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 1536: "1.5 KiB",
		1 << 20: "1.0 MiB", 40 << 30: "40.0 GiB", 3 << 40: "3.0 TiB",
	} {
		assert.Equal(t, want, fmtBytes(n), "%d", n)
	}
}

// The backup lifecycle beside a repaired file: present/absent, delete, and
// the atomic swap back that IS the rollback.
func TestBackupLifecycle(t *testing.T) {
	ctx := t.Context()
	a := &Adapter{}
	dir := t.TempDir()
	path := filepath.Join(dir, "ep.mkv")
	require.NoError(t, os.WriteFile(path, []byte("repaired"), 0o600))

	has, err := a.HasBackup(ctx, path)
	require.NoError(t, err)
	assert.False(t, has)
	err = a.DeleteBackup(ctx, path)
	require.ErrorIs(t, err, mkv.ErrNoBackup)
	err = a.RestoreBackup(ctx, path)
	require.ErrorIs(t, err, mkv.ErrNoBackup)
	got, _ := os.ReadFile(path) //nolint:gosec // test path
	assert.Equal(t, "repaired", string(got), "a failed restore must not touch the file")

	require.NoError(t, os.WriteFile(path+backupSuffix, []byte("original"), 0o600))
	has, err = a.HasBackup(ctx, path)
	require.NoError(t, err)
	assert.True(t, has)

	require.NoError(t, a.RestoreBackup(ctx, path))
	got, _ = os.ReadFile(path) //nolint:gosec // test path
	assert.Equal(t, "original", string(got), "the backup is swapped back in")
	_, err = os.Stat(path + backupSuffix)
	assert.True(t, os.IsNotExist(err), "the swap consumes the backup")

	require.NoError(t, os.WriteFile(path+backupSuffix, []byte("original"), 0o600))
	require.NoError(t, a.DeleteBackup(ctx, path))
	has, _ = a.HasBackup(ctx, path)
	assert.False(t, has)
}
