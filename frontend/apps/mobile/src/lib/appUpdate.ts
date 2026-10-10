import { Linking } from "react-native";
import { newerClientRelease, type ClientRelease } from "@evey/client/native/appUpdate";
import appConfig from "../../app.json";

// The Play Store auto-update waits for an idle device on Wi-Fi and the viewer
// may have turned it off for this app: 10-10, a phone a month on beta.74 while
// beta.79 was out, with a known silent-DTS bug in between. The server
// advertises the phone build released with it on /healthz (clients.mobile);
// this app compares its own build and offers its store page. The versionCode
// here is the app.json one, kept equal to the Gradle one by the release bump
// (and checked by appUpdate.test.ts): it is what the server stamps.

export const INSTALLED_VERSION_CODE: number = appConfig.expo.android.versionCode;
const PACKAGE: string = appConfig.expo.android.package;

export type MobileRelease = ClientRelease;

// The newer phone release a /healthz body advertises, or null: an older
// server, an unstamped build or a malformed field all mean "nothing to offer".
export function newerMobileRelease(health: unknown, installedCode = INSTALLED_VERSION_CODE): MobileRelease | null {
  return newerClientRelease(health, "mobile", installedCode);
}

// The Play Store app, or its web page when no store app answers.
export async function openStorePage(): Promise<void> {
  try {
    await Linking.openURL(`market://details?id=${PACKAGE}`);
  } catch {
    await Linking.openURL(`https://play.google.com/store/apps/details?id=${PACKAGE}`).catch(() => {});
  }
}
