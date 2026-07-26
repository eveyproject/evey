import { Platform } from "react-native";

// Same contract as the mobile app's clientIdentity.ts: a product User-Agent
// on every fetcher request (via setExtraHeaders) so the admin logs and the
// online panel can tell this TV app apart from okhttp's anonymous default.
// The panel maps the "EveyTV" prefix to its TV icon.
export function eveyClientUserAgent(): string {
  const c = (Platform as unknown as { constants?: { Release?: string; Model?: string } }).constants;
  const release = c?.Release ? ` ${c.Release}` : "";
  const model = c?.Model ? `; ${c.Model}` : "";
  return `EveyTV (Android TV${release}${model})`;
}
