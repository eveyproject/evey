/**
 * Same-origin backend artwork URL helpers.
 *
 * Artwork is served with `Cache-Control: immutable, max-age=1y` for speed, but
 * the URL used to be the bare `/artwork/poster` - immutable on a MUTABLE URL.
 * A client that cached an old or 0-byte variant froze it for a year; regenerating
 * server-side never reached it (the browser obeys the header it already cached).
 *
 * Versioning the URL fixes both directions: `?v=<content-version>` makes
 * immutable CORRECT (each version is its own immutable resource), busts the cache
 * the moment the artwork changes, AND recovers already-frozen clients - their
 * cached BARE url is simply never requested again once the app asks for `?v=…`.
 */

type ArtworkVersioned = { updatedAt?: string; createdAt?: string };

/**
 * artworkVersion derives a compact cache-busting token from when the item last
 * changed (updatedAt bumps on a metadata/artwork edit; createdAt as a fallback).
 * Empty when unknown - the URL then stays unversioned for that item rather than
 * inventing a value that would defeat caching on every render.
 */
export function artworkVersion(item: ArtworkVersioned | undefined): string {
  const raw = item?.updatedAt ?? item?.createdAt;
  if (!raw) return "";
  const t = Date.parse(raw);
  return Number.isNaN(t) ? "" : String(t);
}

/**
 * artworkUrl builds `/api/v1/media/<id>/artwork/<kind>` with a `?v=` content
 * version (and an optional auth `token` for the detail views). The server ignores
 * unknown query params and always serves the current artwork, so `?v=` is a
 * pure client-side cache key. Width variants (`&w=`) are appended by callers via
 * the standard separator check, since this URL may already carry a `?`.
 */
export function artworkUrl(
  itemId: string,
  kind: "poster" | "backdrop",
  item: ArtworkVersioned | undefined,
  token?: string,
): string {
  const params: string[] = [];
  const v = artworkVersion(item);
  if (v) params.push(`v=${v}`);
  if (token) params.push(`token=${encodeURIComponent(token)}`);
  const qs = params.length > 0 ? `?${params.join("&")}` : "";
  return `/api/v1/media/${itemId}/artwork/${kind}${qs}`;
}
