import { PermissionsAndroid, Platform } from "react-native";

// The Cast SDK's media notification is posted by a foreground service, and
// that service is what keeps the sender app (and its network) alive once the
// screen turns off. Without POST_NOTIFICATIONS on Android 13+ the notification
// never shows, Android freezes the app ~60s into sleep, the receiver loses its
// sender, and operator boxes (2026-07-26) stop playback instead of
// continuing like the Cast spec says. Asked at cast time, not at boot: the tap
// on the cast button is the natural user-activation moment for the dialog.
export async function ensureCastNotificationPermission(): Promise<void> {
  if (Platform.OS !== "android") return;
  const sdk =
    typeof Platform.Version === "string" ? Number.parseInt(Platform.Version, 10) : Platform.Version;
  // POST_NOTIFICATIONS only exists from API 33; below that the notification
  // shows without any runtime permission.
  if (!sdk || sdk < 33) return;
  const permission = PermissionsAndroid.PERMISSIONS?.POST_NOTIFICATIONS;
  if (!permission) return;
  try {
    if (await PermissionsAndroid.check(permission)) return;
    await PermissionsAndroid.request(permission);
  } catch {
    // A denied or failed dialog must never block the cast itself - the
    // session still works, only the screen-off survival is degraded.
  }
}
