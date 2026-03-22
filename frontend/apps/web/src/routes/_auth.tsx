import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
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
  const { setTokens } = useAuthStore.getState();
  try {
    const res = await fetch("/api/v1/auth/refresh", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    });
    if (!res.ok) return false;
    const data = await res.json() as { accessToken?: string };
    if (data.accessToken) {
      setTokens(data.accessToken);
      return true;
    }
  } catch { /* cookie missing or expired */ }
  return false;
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
