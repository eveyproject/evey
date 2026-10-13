package subtitle

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/subtitle"
)

// replacedDurationToleranceMs is how far a replaced file's duration may move
// before the subtitles downloaded for the old one are judged off-sync: a
// remux of the same cut lands within a few frames, another cut does not.
const replacedDurationToleranceMs = 1000

// downloadedSource is the source of the subtitles this server fetched itself
// (Service.Download). Any other row, and the subtitle files a user put next
// to the media, are the user's and never touched here.
const downloadedSource = "opensubtitles"

// SubscribeReplacedFileCleanup drops the subtitles downloaded for a file that
// was replaced by one of another duration: they were timed on the old cut.
// Kept when the duration holds (within a second) or is unknown. There is no
// automatic subtitle download to queue them again in: the viewer searches
// anew from the picker.
func SubscribeReplacedFileCleanup(bus shared.EventBus, repo subtitle.Repository) {
	bus.Subscribe(library.MediaItemUpdated{}.EventName(), func(ctx context.Context, ev shared.Event) {
		e, ok := ev.(library.MediaItemUpdated)
		if !ok || e.Reason != library.UpdateReasonFileReplaced || !durationMoved(e.PreviousDurationMs, e.DurationMs) {
			return
		}
		dropDownloaded(ctx, repo, string(e.MediaItemID), e.PreviousDurationMs, e.DurationMs)
	})
}

func durationMoved(beforeMs, afterMs int64) bool {
	if beforeMs <= 0 || afterMs <= 0 {
		return false
	}
	d := afterMs - beforeMs
	return d > replacedDurationToleranceMs || d < -replacedDurationToleranceMs
}

func dropDownloaded(ctx context.Context, repo subtitle.Repository, mediaItemID string, beforeMs, afterMs int64) {
	subs, err := repo.GetByMediaItem(ctx, mediaItemID)
	if err != nil {
		slog.WarnContext(ctx, "subtitle: listing the replaced file's subtitles failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return
	}
	dropped := 0
	for _, sub := range subs {
		if sub.Source != downloadedSource {
			continue
		}
		if err := repo.Delete(ctx, sub.ID); err != nil && !errors.Is(err, shared.ErrNotFound) {
			slog.WarnContext(ctx, "subtitle: dropping a subtitle of the replaced file failed",
				slog.String("media_id", mediaItemID), slog.String("subtitle_id", sub.ID), slog.Any("err", err))
			continue
		}
		if err := os.Remove(sub.FilePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.WarnContext(ctx, "subtitle: removing a downloaded subtitle file failed",
				slog.String("path", sub.FilePath), slog.Any("err", err))
		}
		dropped++
	}
	if dropped > 0 {
		slog.InfoContext(ctx, "subtitle: downloaded subtitles of a replaced file of another duration dropped",
			slog.String("media_id", mediaItemID), slog.Int("subtitles", dropped),
			slog.Int64("previous_duration_ms", beforeMs), slog.Int64("duration_ms", afterMs))
	}
}
