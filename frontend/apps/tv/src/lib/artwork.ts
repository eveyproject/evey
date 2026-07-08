// Optional `width` triggers the server-side WebP resize (mirrors web/mobile) -
// drops bandwidth + memory for row/grid thumbnails where 200-400px posters are
// plenty. Omit for full-size hero/backdrop.
export function artworkUrl(
  serverUrl: string,
  mediaId: string,
  type: "poster" | "backdrop" = "poster",
  width?: number,
) {
  const base = `${serverUrl}/api/v1/media/${mediaId}/artwork/${type}`;
  return width && width > 0 ? `${base}?w=${width}` : base;
}
