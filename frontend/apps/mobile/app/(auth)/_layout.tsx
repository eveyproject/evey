import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";
import { DrawerProvider } from "../../src/providers/DrawerProvider";
import { SideDrawer } from "../../src/components/SideDrawer";
import { MiniPlayer } from "../../src/components/MiniPlayer";

export default function AuthLayout() {
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);

  if (!accessToken) return <Redirect href="/login" />;
  if (!profileId) return <Redirect href="/profile-select" />;

  return (
    <DrawerProvider>
      <Stack screenOptions={{ headerShown: false }} />
      <SideDrawer />
      <MiniPlayer />
    </DrawerProvider>
  );
}
