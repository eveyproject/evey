import { createFileRoute, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { userServiceRefreshToken } from "@evey/api";

function isTokenExpired(token: string): boolean {
  try {
    const payload = JSON.parse(atob(token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/")));
    return payload.exp * 1000 < Date.now();
  } catch {
    return true;
  }
}

async function refreshToken(): Promise<boolean> {
  try {
    const data = await userServiceRefreshToken({});
    if (data.accessToken) {
      useAuthStore.getState().setTokens(data.accessToken);
      return true;
    }
    return false;
  } catch {
    return false;
  }
}

export const Route = createFileRoute("/profile-select")({
  beforeLoad: async () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken && !isTokenExpired(accessToken)) return;
    if (await refreshToken()) return;
    throw redirect({ to: "/login" });
  },
});
