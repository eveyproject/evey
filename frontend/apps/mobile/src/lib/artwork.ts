// Optional `width` triggers the server-side WebP resize introduced in web
// commit 3ba78f1e9 - drops bandwidth + memory for list/grid thumbnails
// where 200-400px posters are plenty. Omit for full-size hero/backdrop.
export function artworkUrl(
  serverUrl: string,
  mediaId: string,
  type: "poster" | "backdrop" = "poster",
  width?: number,
) {
  // Encode the id: it can originate from an untrusted deep link, and an
  // unencoded "/" or ".." would redirect the image fetch to an arbitrary
  // API path with the user's session.
  const base = `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/artwork/${type}`;
  return width && width > 0 ? `${base}?w=${width}` : base;
}
