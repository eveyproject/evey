import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { AuthLayout } from "@/components/layout/AuthLayout";

export const Route = createFileRoute("/_auth")({
  beforeLoad: async () => {
    const { accessToken, setTokens } = useAuthStore.getState();
    if (accessToken) return;
    // No in-memory token — try cookie-based refresh before redirecting
    try {
      const res = await fetch("/api/v1/auth/refresh", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      });
      if (!res.ok) throw new Error("refresh failed");
      const data = await res.json() as { accessToken?: string };
      if (data.accessToken) {
        setTokens(data.accessToken);
        return;
      }
    } catch {
      // cookie missing or expired
    }
    throw redirect({ to: "/login" });
  },
  component: () => (
    <AuthLayout>
      <Outlet />
    </AuthLayout>
  ),
});
