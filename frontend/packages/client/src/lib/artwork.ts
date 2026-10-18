// Optional `width` triggers the server-side WebP resize - drops bandwidth and
// memory for row/grid thumbnails where 200-400px posters are plenty. Omit for
// a full-size hero/backdrop. The id is encoded: a path segment it is, a
// literal it is not.
//
// The server serves artwork `immutable` for a year (measured: public,
// max-age=31536000, immutable), so an unversioned URL froze a replaced poster
// in the client's cache for that long. Passing the item adds `?v=<updatedAt>`:
// each version is its own immutable resource, and a change reaches every
// client at once - the web's own copy did this, the shared one did not.
export function artworkUrl(
  serverUrl: string,
  mediaId: string,
  type: "poster" | "backdrop" = "poster",
  width?: number,
  item?: ArtworkVersioned,
) {
  const base = `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/artwork/${type}`;
  const params: string[] = [];
  const v = artworkVersion(item);
  if (v) params.push(`v=${v}`);
  if (width && width > 0) params.push(`w=${width}`);
  return params.length > 0 ? `${base}?${params.join("&")}` : base;
}

export type ArtworkVersioned = { updatedAt?: string; createdAt?: string };

// A compact cache key from when the item last changed (updatedAt bumps on a
// metadata or artwork edit; createdAt as a fallback). Empty when unknown: the
// URL then stays unversioned rather than defeating caching on every render.
export function artworkVersion(item: ArtworkVersioned | undefined): string {
  const raw = item?.updatedAt ?? item?.createdAt;
  if (!raw) return "";
  const t = Date.parse(raw);
  return Number.isNaN(t) ? "" : String(t);
}

// Music cover art comes from the API as a server path (`/api/v1/music/...`)
// or, for provider-hosted art, an absolute URL. The server resizes its own art
// on `?w=` (music_handler parseRequestedWidth) - a 64px row thumbnail must not
// pull the full cover. External art is returned untouched (no resizer there),
// and an existing query string (a signed token) is extended, never clobbered.
export function coverArtUri(serverUrl: string, coverArtUrl?: string, width?: number): string | undefined {
  if (!coverArtUrl) return undefined;
  if (/^https?:/i.test(coverArtUrl)) return coverArtUrl;
  const abs = `${serverUrl}${coverArtUrl}`;
  if (!width || width <= 0) return abs;
  return `${abs}${abs.includes("?") ? "&" : "?"}w=${Math.round(width)}`;
}

/**
 * An image url the server handed over, loadable by a native Image. The
 * server's own paths come relative: they get the server origin (a native
 * Image refuses a relative uri) and the token. An absolute url points at a
 * third party (a collection poster on TMDB, a music cover service) and is
 * returned untouched: the viewer's token must never leave for another host.
 */
export function serverImageUri(serverUrl: string | null | undefined, url: string | undefined, tokenParam: string): string | undefined {
  if (!url) return undefined;
  if (/^https?:/i.test(url)) return url;
  const abs = `${serverUrl ?? ""}${url}`;
  if (!tokenParam) return abs;
  return abs.includes("?") ? `${abs}&${tokenParam.replace(/^\?/, "")}` : `${abs}${tokenParam}`;
}
