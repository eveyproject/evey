import { PixelRatio } from "react-native";

// Shared implementation lives in @evey/client (packages/client/src/lib/artwork.ts).
// This module keeps the path the screens and their tests import.
export { artworkUrl, coverArtUri } from "@evey/client";

// The pixel width to ask the server for, for an image laid out `dp` wide: the
// screen's real density, which the UI scale lowers (80 % by default), not a
// flat x2 - that sent a 1.6-density television 56 % more pixels per poster to
// decode and upload to its GPU.
export function artworkPx(dp: number): number {
  try {
    return PixelRatio.getPixelSizeForLayoutSize(dp);
  } catch {
    // Screen tests mock react-native without PixelRatio: the old flat x2.
    return dp * 2;
  }
}

// TMDB's image CDN serves a face at a few fixed sizes; the server stores the
// "original" one, often well over a thousand pixels tall. A face shown a
// hundred pixels wide is asked at the smallest size that still covers it
// instead of being downloaded and decoded whole. h632 is a height: a 2:3
// portrait at that height is 421 wide. Any other url is left as is.
const TMDB_PROFILE_SIZES: [number, string][] = [[45, "w45"], [185, "w185"], [421, "h632"]];

export function tmdbProfileSized(url: string, px: number): string {
  const m = /^(https:\/\/image\.tmdb\.org\/t\/p\/)original(\/.+)$/.exec(url);
  if (!m) return url;
  const size = TMDB_PROFILE_SIZES.find(([w]) => w >= px)?.[1];
  return size ? `${m[1]}${size}${m[2]}` : url;
}
