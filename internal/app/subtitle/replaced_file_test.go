package subtitle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/subtitle"
	"github.com/eveyproject/evey/internal/domain/subtitle/fakes"
)

// replacedFixture is a film with one subtitle this server downloaded, one a
// user added to the table under another source, and one sidecar file a user
// put next to the media (never in the table).
type replacedFixture struct {
	bus                          *sharedfakes.FakeEventBus
	repo                         *fakes.FakeRepository
	downloaded, userRow, sidecar string
}

func newReplacedFixture(t *testing.T) *replacedFixture {
	t.Helper()
	dir := t.TempDir()
	f := &replacedFixture{
		bus:        sharedfakes.NewFakeEventBus(),
		repo:       fakes.NewFakeRepository(),
		downloaded: filepath.Join(dir, ".subtitles", "film.fr.srt"),
		userRow:    filepath.Join(dir, "mine.en.srt"),
		sidecar:    filepath.Join(dir, "film.de.srt"),
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(f.downloaded), 0o755))
	for _, p := range []string{f.downloaded, f.userRow, f.sidecar} {
		require.NoError(t, os.WriteFile(p, []byte("1\n00:00:01,000 --> 00:00:02,000\nx\n"), 0o644))
	}
	ctx := context.Background()
	require.NoError(t, f.repo.Save(ctx, &subtitle.SubtitleDownload{ID: "sub-dl", MediaItemID: "m1", Language: "fr", Format: "srt", FilePath: f.downloaded, Source: "opensubtitles", DownloadedAt: time.Now()}))
	require.NoError(t, f.repo.Save(ctx, &subtitle.SubtitleDownload{ID: "sub-user", MediaItemID: "m1", Language: "en", Format: "srt", FilePath: f.userRow, Source: "user", DownloadedAt: time.Now()}))
	SubscribeReplacedFileCleanup(f.bus, f.repo)
	return f
}

func (f *replacedFixture) replace(t *testing.T, beforeMs, afterMs int64) {
	t.Helper()
	require.NoError(t, f.bus.Publish(context.Background(), library.MediaItemUpdated{
		MediaItemID: shared.MediaItemID("m1"), Reason: library.UpdateReasonFileReplaced,
		PreviousDurationMs: beforeMs, DurationMs: afterMs,
	}))
}

func (f *replacedFixture) ids(t *testing.T) []string {
	t.Helper()
	subs, err := f.repo.GetByMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	var out []string
	for _, s := range subs {
		out = append(out, s.ID)
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Another cut (the duration moved by more than a second): the downloaded
// subtitle was timed on the old one and goes, row and file.
func TestReplacedFile_OtherDurationDropsTheDownloadedSubtitles(t *testing.T) {
	f := newReplacedFixture(t)
	f.replace(t, 5_400_000, 5_401_001)
	assert.NotContains(t, f.ids(t), "sub-dl")
	assert.False(t, exists(f.downloaded))
}

// The same cut (within a second), or a duration unknown on either side: kept.
func TestReplacedFile_SameDurationKeepsThem(t *testing.T) {
	for _, d := range [][2]int64{{5_400_000, 5_401_000}, {5_400_000, 5_399_000}, {5_400_000, 5_400_040}, {0, 5_400_000}, {5_400_000, 0}} {
		f := newReplacedFixture(t)
		f.replace(t, d[0], d[1])
		assert.Contains(t, f.ids(t), "sub-dl", "durations %v", d)
		assert.True(t, exists(f.downloaded), "durations %v", d)
	}
	// Another reason than a replaced file changes nothing either.
	f := newReplacedFixture(t)
	require.NoError(t, f.bus.Publish(context.Background(), library.MediaItemUpdated{
		MediaItemID: "m1", Reason: library.UpdateReasonMetadataEdited, PreviousDurationMs: 1, DurationMs: 9_999_999,
	}))
	assert.Contains(t, f.ids(t), "sub-dl")
}

// What a user added is never touched, even when the duration moved: a row
// under another source and a sidecar file next to the media.
func TestReplacedFile_UserSubtitlesAreNeverTouched(t *testing.T) {
	f := newReplacedFixture(t)
	f.replace(t, 5_400_000, 7_200_000)
	assert.Equal(t, []string{"sub-user"}, f.ids(t))
	assert.True(t, exists(f.userRow))
	assert.True(t, exists(f.sidecar))
}
