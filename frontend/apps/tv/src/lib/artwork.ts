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
