// Shared implementation lives in @evey/client (packages/client/src/native/useDeviceRegistration.ts).
// This module keeps the path the screens and their tests import, and the
// television's policy: a generic name, the active server from the store, and
// a socket that stays open - a TV does not go to the background.
import { useDeviceRegistration as useSharedDeviceRegistration } from "@evey/client/native/useDeviceRegistration";
import { Platform } from "react-native";
import { useRouter } from "expo-router";
import { useServerListStore } from "@evey/store";

function deviceName(): string {
  if (Platform.OS === "ios") return "Apple TV";
  if (Platform.OS === "android") return "Android TV";
  return "TV";
}

export function useDeviceRegistration() {
  const router = useRouter();
  const serverUrl = useServerListStore((s) => s.getActiveServer()?.url);
  useSharedDeviceRegistration({
    deviceType: "tv", deviceName, serverUrl, pauseInBackground: false,
    onLoadAndPlay: (mediaId, positionMs) =>
      router.push({
        pathname: "/(auth)/player/[id]",
        params: positionMs ? { id: mediaId, positionMs: String(positionMs) } : { id: mediaId },
      }),
  });
}
