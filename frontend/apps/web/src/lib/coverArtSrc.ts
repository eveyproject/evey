// The URL of a music cover for a sized slot. Server-hosted art
// (`/api/v1/music/...`) takes `?w=` and comes back as a display-sized WebP;
// provider-hosted art (absolute http(s)) is returned untouched - nothing
// resizes it. The signed token, when present, is already a query string, so
// the width is appended with the right separator.
export function coverArtSrc(url: string | undefined, tokenParam: string, width: number): string | undefined {
  if (!url) return undefined;
  if (/^https?:/i.test(url)) return url;
  const base = `${url}${tokenParam}`;
  if (!width || width <= 0) return base;
  return `${base}${base.includes("?") ? "&" : "?"}w=${Math.round(width)}`;
}
