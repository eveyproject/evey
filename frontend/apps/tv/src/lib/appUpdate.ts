import { Linking } from "react-native";
import { newerClientRelease, type ClientRelease } from "@evey/client/native/appUpdate";
import appConfig from "../../app.json";

// The Play Store auto-update rarely runs on a television: its jobs wait for an
// idle device, and a TV is either in use or asleep (27-09, a TV stuck a week on
// 0.1.4). The server advertises the TV build released with it on /healthz;
// this app compares its own build and sends the viewer to its store page.
// The comparison is shared with the phone (@evey/client/native/appUpdate).

export const INSTALLED_VERSION_CODE: number = appConfig.expo.android.versionCode;
const PACKAGE: string = appConfig.expo.android.package;

export type TVRelease = ClientRelease;

// The newer TV release a /healthz body advertises, or null: an older server,
// an unstamped build or a malformed field all mean "nothing to offer".
export function newerTVRelease(health: unknown, installedCode = INSTALLED_VERSION_CODE): TVRelease | null {
  return newerClientRelease(health, "tv", installedCode);
}

// The Play Store app on the TV, or its web page when no store app answers.
export async function openStorePage(): Promise<void> {
  try {
    await Linking.openURL(`market://details?id=${PACKAGE}`);
  } catch {
    await Linking.openURL(`https://play.google.com/store/apps/details?id=${PACKAGE}`).catch(() => {});
  }
}
