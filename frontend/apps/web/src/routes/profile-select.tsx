import { createFileRoute, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { isTokenExpired, refreshTokenUnderLock } from "@/lib/authRefresh";

export const Route = createFileRoute("/profile-select")({
  beforeLoad: async () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken && !isTokenExpired(accessToken)) return;
    if (await refreshTokenUnderLock()) return;
    throw redirect({ to: "/login" });
  },
});
