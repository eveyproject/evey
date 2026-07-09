import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { AuthLayout } from "@/components/layout/AuthLayout";
import { ImpersonationBanner } from "@/components/layout/ImpersonationBanner";
import { PresenceHeartbeat } from "@/components/PresenceHeartbeat";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";
import { isTokenExpired, refreshTokenUnderLock } from "@/lib/authRefresh";

export const Route = createFileRoute("/_auth")({
  beforeLoad: async () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken && !isTokenExpired(accessToken)) return;
    if (await refreshTokenUnderLock()) return;
    // Refresh failed: the session is dead. Clear the stale token before
    // redirecting — otherwise index/login still see a truthy accessToken and
    // bounce the user through extra redirects before a 401 finally logs out.
    useAuthStore.getState().logout();
    throw redirect({ to: "/login" });
  },
  errorComponent: RouteErrorComponent,
  component: () => (
    <AuthLayout>
      <PresenceHeartbeat />
      <ImpersonationBanner />
      <Outlet />
    </AuthLayout>
  ),
});
