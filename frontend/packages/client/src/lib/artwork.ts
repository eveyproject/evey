// Optional `width` triggers the server-side WebP resize - drops bandwidth and
// memory for row/grid thumbnails where 200-400px posters are plenty. Omit for
// a full-size hero/backdrop. The id is encoded: a path segment it is, a
// literal it is not.
export function artworkUrl(
  serverUrl: string,
  mediaId: string,
  type: "poster" | "backdrop" = "poster",
  width?: number,
) {
  const base = `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/artwork/${type}`;
  return width && width > 0 ? `${base}?w=${width}` : base;
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
