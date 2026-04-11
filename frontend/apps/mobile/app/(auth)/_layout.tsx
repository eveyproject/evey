import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";
import { DrawerProvider } from "../../src/providers/DrawerProvider";
import { SideDrawer } from "../../src/components/SideDrawer";
import { MiniPlayer } from "../../src/components/MiniPlayer";

function isTokenExpired(token: string): boolean {
  try {
    const payload = JSON.parse(atob(token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/")));
    return typeof payload.exp === "number" && payload.exp < Date.now() / 1000;
  } catch {
    return true;
  }
}

export default function AuthLayout() {
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const logout = useAuthStore((s) => s.logout);

  if (!accessToken || isTokenExpired(accessToken)) {
    if (accessToken) logout();
    return <Redirect href="/login" />;
  }
  if (!profileId) return <Redirect href="/profile-select" />;

  return (
    <DrawerProvider>
      <Stack screenOptions={{ headerShown: false }} />
      <SideDrawer />
      <MiniPlayer />
    </DrawerProvider>
  );
}
