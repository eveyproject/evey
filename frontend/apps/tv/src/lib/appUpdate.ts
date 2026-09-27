import { Linking } from "react-native";
import appConfig from "../../app.json";

// The Play Store auto-update rarely runs on a television: its jobs wait for an
// idle device, and a TV is either in use or asleep (27-09, a TV stuck a week on
// 0.1.4). The server advertises the TV build released with it on /healthz;
// this app compares its own build and sends the viewer to its store page.

export const INSTALLED_VERSION_CODE: number = appConfig.expo.android.versionCode;
const PACKAGE: string = appConfig.expo.android.package;

export interface TVRelease {
  version: string;
  versionCode: number;
}

// The newer TV release a /healthz body advertises, or null: an older server,
// an unstamped build or a malformed field all mean "nothing to offer".
export function newerTVRelease(health: unknown, installedCode = INSTALLED_VERSION_CODE): TVRelease | null {
  const tv = (health as { clients?: { tv?: { version?: unknown; version_code?: unknown } } } | null)?.clients?.tv;
  const code = tv?.version_code;
  if (typeof code !== "number" || !Number.isInteger(code) || code <= installedCode) return null;
  return { version: typeof tv?.version === "string" ? tv.version : String(code), versionCode: code };
}

// The Play Store app on the TV, or its web page when no store app answers.
export async function openStorePage(): Promise<void> {
  try {
    await Linking.openURL(`market://details?id=${PACKAGE}`);
  } catch {
    await Linking.openURL(`https://play.google.com/store/apps/details?id=${PACKAGE}`).catch(() => {});
  }
}
