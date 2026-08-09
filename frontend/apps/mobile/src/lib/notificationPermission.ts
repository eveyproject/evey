import { PermissionsAndroid, Platform } from "react-native";

// Two features post foreground-service notifications and both are invisible
// on Android 13+ until POST_NOTIFICATIONS is granted:
// - the Cast SDK's media notification, whose service keeps the sender app
//   (and its network) alive once the screen turns off - without it Android
//   freezes the app ~60s into sleep, the receiver loses its sender, and
//   operator boxes (2026-07-26) stop playback;
// - the background downloader's progress notification (the service itself
//   still runs, but the user sees no progress and no completion).
// Asked at cast/download time, not at boot: that tap is the natural
// user-activation moment for the dialog.
export async function ensureNotificationPermission(): Promise<void> {
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
