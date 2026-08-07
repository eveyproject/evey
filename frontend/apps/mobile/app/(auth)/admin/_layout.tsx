import { View } from "react-native";
import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";
import { NavRail } from "../../../src/components/layout/NavRail";

// Admin screens sit beside the permanent icon rail (web sidebar collapsed
// state, admin sections flattened) - one tap to any admin area or back to the
// app; the brand mark expands the full labelled drawer. Detail screens (user
// edit, media segments, plugin extension) override headerLeft with HeaderBack
// to return to their caller.
export default function AdminLayout() {
  const isAdmin = useAuthStore((s) => s.isAdmin);
  if (!isAdmin) return <Redirect href="/(auth)/(tabs)" />;
  return (
    <View style={{ flex: 1, flexDirection: "row" }}>
      <NavRail mode="admin" />
      <View style={{ flex: 1 }}>
        <Stack screenOptions={{ headerShown: false, headerLeft: () => null }} />
      </View>
    </View>
  );
}
