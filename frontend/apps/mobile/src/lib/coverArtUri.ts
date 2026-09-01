import { PixelRatio } from "react-native";

// Music cover art for a sized slot. Server paths (`/api/v1/music/...`) get
// the active server prefixed (expo-image refuses relative URIs) and a `?w=`
// at the device's pixel density - the server answers with a display-sized
// WebP; provider-hosted art (absolute http(s)) is returned untouched. An
// existing query string (a signed token) is extended, never clobbered.
export function coverArtUri(serverUrl: string | undefined, coverArtUrl: string | undefined, widthDp: number): string | undefined {
  if (!coverArtUrl) return undefined;
  if (/^https?:/i.test(coverArtUrl)) return coverArtUrl;
  const abs = `${serverUrl ?? ""}${coverArtUrl}`;
  const px = Math.round(widthDp * densityScale());
  if (px <= 0) return abs;
  return `${abs}${abs.includes("?") ? "&" : "?"}w=${px}`;
}

// The test double of react-native has no PixelRatio; 2x is the common phone.
function densityScale(): number {
  try {
    return PixelRatio?.get?.() ?? 2;
  } catch {
    return 2;
  }
}
