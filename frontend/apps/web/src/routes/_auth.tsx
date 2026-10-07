import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore, loadProfileCache, shouldPickProfileOnColdStart } from "@evey/store";
import { AuthLayout } from "@/components/layout/AuthLayout";
import { ImpersonationBanner } from "@/components/layout/ImpersonationBanner";
import { DemoBanner } from "@/components/layout/DemoBanner";
import { PresenceHeartbeat } from "@/components/PresenceHeartbeat";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";
import { isTokenExpired, refreshTokenWithOutcome } from "@/lib/authRefresh";
import { ProfileCacheSync } from "@/components/ProfileCacheSync";

// "Who is watching", asked once per DOCUMENT LOAD - the browser's equivalent of
// the cold start the phone and the television use. A module-level flag is what
// draws that line: it resets when the page is loaded or reloaded, and survives
// every in-app navigation, so moving between screens never re-asks.
//
// A tab that has been sitting open on an adult profile is not covered, exactly
// as a phone left warm is not. The trade was made deliberately: the protection
// is the PIN on LEAVING a restricted profile, and this screen is what makes
// handing the device over an explicit act rather than a default.
let coldStartHandled = false;

export const Route = createFileRoute("/_auth")({
  beforeLoad: async ({ location }) => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken && !isTokenExpired(accessToken)) {
      await maybePickProfile(location.pathname);
      return;
    }
    const outcome = await refreshTokenWithOutcome();
    if (outcome === "ok") {
      await maybePickProfile(location.pathname);
      return;
    }
    // The server could not be asked (offline, 5xx): the session is not known
    // to be dead, so keep it. The screen renders, its requests fail visibly and
    // retry, and the fetcher's own 401 path signs out if the server says so.
    if (outcome === "transient") return;
    // Refresh denied: the session is dead. Clear the stale token before
    // redirecting - otherwise index/login still see a truthy accessToken and
    // bounce the user through extra redirects before a 401 finally logs out.
    useAuthStore.getState().logout();
    throw redirect({ to: "/login" });
  },
  errorComponent: RouteErrorComponent,
  component: () => (
    <AuthLayout>
      <ProfileCacheSync />
      <PresenceHeartbeat />
      <ImpersonationBanner />
      <DemoBanner />
      <Outlet />
    </AuthLayout>
  ),
});

// Only where somebody could pick wrong: a household with a kids profile AND
// more than one profile. Everyone else never sees this screen. The rule and
// the cache live in @evey/store so the browser cannot drift from the devices.
async function maybePickProfile(pathname: string): Promise<void> {
  if (coldStartHandled) return;
  coldStartHandled = true;
  // Landing ON the picker already needs no redirect to it.
  if (pathname.startsWith("/profile-select")) return;
  if (!useAuthStore.getState().profileId) return;
  if (shouldPickProfileOnColdStart(await loadProfileCache())) {
    throw redirect({ to: "/profile-select" });
  }
}
