import { getBaseURL } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { artworkUrl } from "@evey/client";

// An <img> cannot send a header: the access token rides in the query, as
// the phone's image loader does (the token middleware reads it there).
export function posterSrc(mediaId: string | undefined, type: "poster" | "backdrop" = "poster", width = 400): string | undefined {
  if (!mediaId) return undefined;
  const token = useAuthStore.getState().accessToken;
  const url = artworkUrl(getBaseURL(), mediaId, type, width);
  return token ? `${url}${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : url;
}

export function isEpisode(item: { type?: string } | undefined): boolean {
  return item?.type === "MEDIA_ITEM_TYPE_EPISODE";
}

export function isSeries(item: { type?: string } | undefined): boolean {
  return item?.type === "MEDIA_ITEM_TYPE_SERIES";
}

export function isSeason(item: { type?: string } | undefined): boolean {
  return item?.type === "MEDIA_ITEM_TYPE_SEASON";
}

/** A playable item: a film or an episode; a series or a season opens a list. */
export function isPlayable(item: { type?: string } | undefined): boolean {
  return !isSeries(item) && !isSeason(item);
}
