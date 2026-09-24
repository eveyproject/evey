import { Platform } from "react-native";
import { clientUserAgent } from "@evey/client";
import appConfig from "../../app.json";

// Same contract as the mobile app's clientIdentity.ts: a product User-Agent
// on every fetcher request (via setExtraHeaders) so the admin logs and the
// online panel can tell this TV app apart from okhttp's anonymous default.
// The build rides in it like the phone's: without it a log line could not say
// whether a television ran the fixed build. The version is app.json's, bundled
// with the JavaScript (this app does not ship expo-constants).
export function eveyClientUserAgent(): string {
  const c = (Platform as unknown as { constants?: { Release?: string; Model?: string } }).constants;
  return clientUserAgent({ product: "EveyTV", version: appConfig.expo.version, os: "Android TV", release: c?.Release, model: c?.Model });
}
