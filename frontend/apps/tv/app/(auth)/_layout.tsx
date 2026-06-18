import { View } from "react-native";
import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";
import { useDeviceRegistration } from "../../src/hooks/useDeviceRegistration";
import { TVMusicPlayer } from "../../src/components/TVMusicPlayer";

function isTokenExpired(token: string): boolean {
  try {
    const tokenParts = token.split(".");
    if (tokenParts.length < 3 || !tokenParts[1]) return true;
    const payload = JSON.parse(atob(tokenParts[1].replace(/-/g, "+").replace(/_/g, "/")));
    return typeof payload.exp === "number" && payload.exp < Date.now() / 1000;
  } catch {
    return true;
  }
}

export default function AuthLayout() {
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const logout = useAuthStore((s) => s.logout);
  // Register this client as a Play On target. No-ops when not
  // authenticated (the hook bails if accessToken/serverUrl are absent),
  // so calling it before the redirect-on-no-auth guard is safe.
  useDeviceRegistration();

  if (!accessToken || isTokenExpired(accessToken)) {
    if (accessToken) logout();
    return <Redirect href="/login" />;
  }
  if (!profileId) return <Redirect href="/profile-select" />;

  return (
    <View style={{ flex: 1 }}>
      <Stack screenOptions={{ headerShown: false }} />
      <TVMusicPlayer />
    </View>
  );
}
