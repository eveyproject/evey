import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";
import { BurgerButton } from "../../../src/components/layout/BurgerButton";

// Top-level admin screens show the drawer burger instead of a back arrow -
// same escape hatch as the web sidebar (the drawer carries the admin sections
// and "back to app"). Detail screens (user edit, media segments, plugin
// extension) override headerLeft with HeaderBack to return to their caller.
export default function AdminLayout() {
  const isAdmin = useAuthStore((s) => s.isAdmin);
  if (!isAdmin) return <Redirect href="/(auth)/(tabs)" />;
  return <Stack screenOptions={{ headerShown: false, headerLeft: () => <BurgerButton /> }} />;
}
