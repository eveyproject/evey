import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { userServiceRefreshToken } from "@evey/api";
import { AuthLayout } from "@/components/layout/AuthLayout";
import { ImpersonationBanner } from "@/components/layout/ImpersonationBanner";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";

function isTokenExpired(token: string): boolean {
  try {
    const tokenParts = token.split(".");
    if (tokenParts.length < 3 || !tokenParts[1]) return true;
    const payload = JSON.parse(atob(tokenParts[1].replace(/-/g, "+").replace(/_/g, "/")));
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
  } catch (err) {
    console.warn("token refresh failed", err);
    return false;
  }
}

export const Route = createFileRoute("/_auth")({
  beforeLoad: async () => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken && !isTokenExpired(accessToken)) return;
    if (await refreshToken()) return;
    // Refresh failed: the session is dead. Clear the stale token before
    // redirecting — otherwise index/login still see a truthy accessToken and
    // bounce the user through extra redirects before a 401 finally logs out.
    useAuthStore.getState().logout();
    throw redirect({ to: "/login" });
  },
  errorComponent: RouteErrorComponent,
  component: () => (
    <AuthLayout>
      <ImpersonationBanner />
      <Outlet />
    </AuthLayout>
  ),
});
