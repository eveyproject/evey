import { Alert, Platform } from "react-native";

import { isMeteredNetwork } from "./networkHint";

type T = (key: string, fallback: string) => string;

/**
 * Opens Android's floating internet panel, where Wi-Fi can be switched on
 * without leaving Evey. Turning Wi-Fi on programmatically has been forbidden
 * since Android 10, so handing the user the panel is the closest we get.
 * Falls back to the full Wi-Fi settings screen on anything that rejects the
 * panel intent, and does nothing at all on iOS (no equivalent exists there -
 * silently failing is better than a button that appears to do nothing).
 */
export async function openWifiSettings(): Promise<void> {
  if (Platform.OS !== "android") return;
  // Imported HERE, not at module scope: expo-intent-launcher pulls in
  // expo-modules-core, which needs the native globalThis.expo that no test
  // environment provides. A top-level import made an unrelated screen test
  // blow up with "Cannot read properties of undefined (reading
  // 'EventEmitter')" - and would have done so for every future test touching
  // a screen that can download. Loading it on tap keeps that cost where the
  // native module is actually needed.
  const IntentLauncher = await import("expo-intent-launcher");
  try {
    await IntentLauncher.startActivityAsync("android.settings.panel.action.INTERNET_CONNECTIVITY");
  } catch {
    try {
      await IntentLauncher.startActivityAsync(IntentLauncher.ActivityAction.WIFI_SETTINGS);
    } catch {
      // Both refused: the device has no settings activity we can reach. The
      // caller still gets its answer, the download just proceeds on data.
    }
  }
}

/**
 * Asked BEFORE a download starts, and only on mobile data. Resolves true when
 * the download should go ahead.
 *
 * This is a question, not a gate: "Download anyway" is always available,
 * because downloading on cellular is a legitimate choice - a train, a hotel, a
 * home connection that is down. What it prevents is the accidental gigabyte,
 * which is the actual complaint. On Wi-Fi it never appears at all.
 */
export async function confirmDownloadOnMobileData(t: T): Promise<boolean> {
  if (!(await isMeteredNetwork())) return true;
  return new Promise<boolean>((resolve) => {
    Alert.alert(
      t("media.detail.mobileDataTitle", "You are on mobile data"),
      t("media.detail.mobileDataBody", "This download may use several gigabytes. Wi-Fi is recommended."),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel", onPress: () => resolve(false) },
        {
          // Opening the panel does NOT start the download: the user still has
          // to pick a network, and we cannot know when they are done. They come
          // back and tap download again, now on Wi-Fi, and see no dialog.
          text: t("media.detail.turnOnWifi", "Turn on Wi-Fi"),
          onPress: () => {
            void openWifiSettings();
            resolve(false);
          },
        },
        { text: t("media.detail.downloadAnyway", "Download anyway"), onPress: () => resolve(true) },
      ],
      { cancelable: true, onDismiss: () => resolve(false) },
    );
  });
}
