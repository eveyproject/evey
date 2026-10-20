import type { V1WatchProgress } from "./generated/playback.schemas";
import { episodeLabel } from "./episodeLabel";

/**
 * GetWatchHistory populates title / seriesTitle / indexNumber so clients can
 * show a real label instead of the bare media_item_id. Web, mobile and TV each
 * grew their own copy of this formatting and drifted (the TV app still printed
 * `mediaItemId.slice(0, 8)`), so it lives here once, on top of episodeLabel.
 *
 * Returns null when the server could not resolve the item - the caller supplies
 * its own localized fallback, since the i18n `t` signature differs between the
 * web and React Native apps.
 */
export function watchHistoryLabel(item: V1WatchProgress): string | null {
  const title = item.title?.trim();
  if (item.mediaType === "episode") {
    const label = episodeLabel({
      seriesTitle: item.seriesTitle,
      seasonNumber: item.parentIndexNumber,
      episodeNumber: item.indexNumber,
      title,
    });
    if (label) return label;
  }
  return title || null;
}
