import { getBaseURL } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { artworkUrl } from "@evey/client";

// The artwork route is public: a poster URL carries no token (in the query it
// only reached the proxy logs, and changing at each refresh it defeated the
// browser cache).
export function posterSrc(mediaId: string | undefined, type: "poster" | "backdrop" = "poster", width = 400): string | undefined {
  if (!mediaId) return undefined;
  return artworkUrl(getBaseURL(), mediaId, type, width);
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

/** A server path the browser loads itself (an <img>): the token rides the query. */
export function authedSrc(path: string): string {
  const token = useAuthStore.getState().accessToken;
  const url = `${getBaseURL()}${path}`;
  return token ? `${url}${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : url;
}

export function avatarSrc(profileId: string): string {
  return authedSrc(`/api/v1/profiles/${profileId}/avatar`);
}

/**
 * An image url the server handed over, loadable by an <img>. A server-relative
 * path gets the token; an absolute url is another host's (a collection poster
 * on TMDB) and is used as is: the viewer's token must never leave for it.
 */
export function serverImageSrc(url: string | undefined): string | undefined {
  if (!url) return undefined;
  return /^https?:/i.test(url) ? url : authedSrc(url);
}
