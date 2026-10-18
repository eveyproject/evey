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

// The version key is @evey/client's, shared with the phone and the
// televisions (their artwork URLs carry it too since 2026-09-24).
import { artworkVersion, type ArtworkVersioned } from "@evey/client";

export { artworkVersion };

/**
 * artworkUrl builds `/api/v1/media/<id>/artwork/<kind>` with a `?v=` content
 * version. The route is public: no token rides the URL, where it only reached
 * the proxy logs and the browser history. The server ignores
 * unknown query params and always serves the current artwork, so `?v=` is a
 * pure client-side cache key. Width variants (`&w=`) are appended by callers via
 * the standard separator check, since this URL may already carry a `?`.
 */
export function artworkUrl(
  itemId: string,
  kind: "poster" | "backdrop",
  item: ArtworkVersioned | undefined,
): string {
  const params: string[] = [];
  const v = artworkVersion(item);
  if (v) params.push(`v=${v}`);
  const qs = params.length > 0 ? `?${params.join("&")}` : "";
  return `/api/v1/media/${itemId}/artwork/${kind}${qs}`;
}
