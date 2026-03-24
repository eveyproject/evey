import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { AuthLayout } from "@/components/layout/AuthLayout";

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
    const data = await eveyFetch<{ accessToken?: string }>({
      url: "/api/v1/auth/refresh",
      method: "POST",
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    return !!data.accessToken;
  } catch {
    return false;
  }
}

export const Route = createFileRoute("/_auth")({
  beforeLoad: async () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken && !isTokenExpired(accessToken)) return;
    if (await refreshToken()) return;
    throw redirect({ to: "/login" });
  },
  component: () => (
    <AuthLayout>
      <Outlet />
    </AuthLayout>
  ),
});
