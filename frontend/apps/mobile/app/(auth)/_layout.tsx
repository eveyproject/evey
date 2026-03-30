import { Redirect, Stack } from "expo-router";
import { useAuthStore } from "@evey/store";

export default function AuthLayout() {
  const accessToken = useAuthStore((s) => s.accessToken);

  if (!accessToken) return <Redirect href="/login" />;

  return <Stack screenOptions={{ headerShown: false }} />;
}
