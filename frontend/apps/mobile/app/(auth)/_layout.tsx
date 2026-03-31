import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";

export default function AuthLayout() {
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);

  if (!accessToken) return <Redirect href="/login" />;
  if (!profileId) return <Redirect href="/profile-select" />;

  return <Stack screenOptions={{ headerShown: false }} />;
}
