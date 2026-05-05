// Optional `width` triggers the server-side WebP resize introduced in web
// commit 3ba78f1e9 — drops bandwidth + memory for list/grid thumbnails
// where 200-400px posters are plenty. Omit for full-size hero/backdrop.
export function artworkUrl(
  serverUrl: string,
  mediaId: string,
  type: "poster" | "backdrop" = "poster",
  width?: number,
) {
  const base = `${serverUrl}/api/v1/media/${mediaId}/artwork/${type}`;
  return width && width > 0 ? `${base}?w=${width}` : base;
}
