import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";

export default function AdminLayout() {
  const isAdmin = useAuthStore((s) => s.isAdmin);
  if (!isAdmin) return <Redirect href="/(auth)/(tabs)" />;
  return <Stack screenOptions={{ headerShown: false }} />;
}
