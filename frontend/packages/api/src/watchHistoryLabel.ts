import type { V1WatchProgress } from "./generated/playback.schemas";

/**
 * GetWatchHistory populates title / seriesTitle / indexNumber so clients can
 * show a real label instead of the bare media_item_id. Web, mobile and TV each
 * grew their own copy of this formatting and drifted (the TV app still printed
 * `mediaItemId.slice(0, 8)`), so it lives here once.
 *
 * Returns null when the server could not resolve the item - the caller supplies
 * its own localized fallback, since the i18n `t` signature differs between the
 * web and React Native apps.
 */
export function watchHistoryLabel(item: V1WatchProgress): string | null {
  const title = item.title?.trim();
  if (item.mediaType === "episode") {
    const s = item.parentIndexNumber;
    const e = item.indexNumber;
    const code = s && e ? `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}` : e ? `E${e}` : "";
    const parts = [item.seriesTitle?.trim(), code, title].filter(Boolean);
    if (parts.length) return parts.join(" · ");
  }
  return title || null;
}
