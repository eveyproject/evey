import { Platform } from "react-native";

// Every API request used to reach the server as "okhttp/4.12.0" - unreadable
// in the admin logs (the player-event handler logs the raw User-Agent as
// `device`) and rendered with a desktop icon on the online panel. This product
// UA rides on all fetcher requests via setExtraHeaders (okhttp and the iOS
// stack both honor a User-Agent header override). The TV app ships the same
// shape as "EveyTV (...)" so the panel can tell the three clients apart.
export function eveyClientUserAgent(): string {
  if (Platform.OS === "android") {
    const c = (Platform as unknown as { constants?: { Release?: string; Model?: string } }).constants;
    const release = c?.Release ? ` ${c.Release}` : "";
    const model = c?.Model ? `; ${c.Model}` : "";
    return `EveyMobile (Android${release}${model})`;
  }
  return `EveyMobile (iOS ${Platform.Version})`;
}
