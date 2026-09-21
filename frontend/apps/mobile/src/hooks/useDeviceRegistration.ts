// Shared implementation lives in @evey/client (packages/client/src/native/useDeviceRegistration.ts).
// This module keeps the path the screens and their tests import, and the
// phone's policy: its OS name in the Play On picker, and a socket that closes
// while the app is in the background.
import { useDeviceRegistration as useSharedDeviceRegistration } from "@evey/client/native/useDeviceRegistration";
import Constants from "expo-constants";
import { Platform } from "react-native";
import { useRouter } from "expo-router";
import { useServerUrl } from "./useServerUrl";

// Prefer the OS-reported device name (e.g. "Alice's iPhone") so the Play On
// picker can tell a user's own phones apart - falling back to a generic
// label when the platform doesn't expose one (no expo-device dependency
// available in this app; expo-constants already ships deviceName).
function deviceName(): string {
  if (Constants.deviceName) return Constants.deviceName;
  if (Platform.OS === "ios") return "iPhone";
  if (Platform.OS === "android") return "Android Phone";
  return "Mobile";
}

export function useDeviceRegistration() {
  const router = useRouter();
  const serverUrl = useServerUrl();
  useSharedDeviceRegistration({
    deviceType: "mobile", deviceName, serverUrl, pauseInBackground: true,
    onLoadAndPlay: (mediaId, positionMs) =>
      router.push({
        pathname: "/(auth)/player/[id]",
        params: positionMs ? { id: mediaId, positionMs: String(positionMs) } : { id: mediaId },
      }),
  });
}
