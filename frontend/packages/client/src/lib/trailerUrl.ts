import type { V1Trailer } from "@evey/api";

// A trailer is stored as a site + key, never a URL, so every client builds the
// link itself. Kept identical to mobile's TrailerButton so a site one client
// can open is not silently missing on another.
//
// Returns null for anything we cannot build a link for: a button that leads
// nowhere is worse on a remote than an absent one, where the D-pad would land
// on a control that does nothing.
export function trailerUrl(trailer: V1Trailer): string | null {
  if (!trailer.key || !trailer.site) return null;
  switch (trailer.site.toLowerCase()) {
    case "youtube":
      return `https://www.youtube.com/watch?v=${trailer.key}`;
    case "vimeo":
      return `https://vimeo.com/${trailer.key}`;
    default:
      return null;
  }
}

// Trailers ordered the way the viewer expects to find them: the official one
// first, then anything explicitly typed "Trailer", then the rest.
export function sortTrailers(trailers: V1Trailer[]): V1Trailer[] {
  const rank = (tr: V1Trailer) => (tr.official ? 0 : 1) + (tr.type === "Trailer" ? 0 : 1);
  return [...trailers].sort((a, b) => rank(a) - rank(b));
}
