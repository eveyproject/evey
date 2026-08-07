import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";
import { RailFrame } from "../../../src/components/layout/RailFrame";

// Admin screens sit beside the auto-hiding icon rail (web sidebar collapsed
// state, admin sections flattened) - one tap to any admin area or back to the
// app; the brand mark expands the full labelled drawer. Detail screens (user
// edit, media segments, plugin extension) override headerLeft with HeaderBack
// to return to their caller.
export default function AdminLayout() {
  const isAdmin = useAuthStore((s) => s.isAdmin);
  if (!isAdmin) return <Redirect href="/(auth)/(tabs)" />;
  return (
    <RailFrame mode="admin">
      <Stack screenOptions={{ headerShown: false, headerLeft: () => null }} />
    </RailFrame>
  );
}
