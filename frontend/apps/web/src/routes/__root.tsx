import { useEffect, useRef } from "react";
import { createRootRouteWithContext, Outlet, redirect, useRouterState } from "@tanstack/react-router";
import { type QueryClient, useQuery } from "@tanstack/react-query";
import { adminSystemServiceGetSetupStatus, activityServiceRecordPageView } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import i18n, { supportedLanguages } from "@/i18n";
import { ThemeProvider } from "@/components/ThemeProvider";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";
import { ToastContainer } from "@/components/ui/ToastContainer";

interface RouterContext {
  queryClient: QueryClient;
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ context, location }) => {
    try {
      const info = await context.queryClient.fetchQuery<{
        setupComplete?: boolean;
        serverLanguage?: string;
        serverRegion?: string;
      }>({
        queryKey: ["setup-status"],
        queryFn: () => adminSystemServiceGetSetupStatus(),
        staleTime: 60_000,
      });
      // Apply server's default language on unauthenticated sessions so /login
      // and /setup render in the admin-picked language instead of the browser
      // default. Once the user logs in, their profile locale (JWT claim) and
      // any explicit /settings choice take over via the existing listeners.
      if (info.serverLanguage && !useAuthStore.getState().accessToken) {
        const { locale: current, source } = useLocaleStore.getState();
        // Precedence decided 2026-08-05: profile > user pick > device >
        // server default. A profile claim or an explicit pick outranks the
        // server default, and so does a SUPPORTED browser language - the
        // unmarked store value is seeded from navigator.language, which is
        // the web's "device" tier (mirrors the mobile/TV check).
        const browserLang = (typeof navigator !== "undefined" ? navigator.language : "")?.split("-")[0];
        const browserSupported = supportedLanguages.some((l) => l.code === browserLang);
        if (
          source !== "user" &&
          source !== "profile" &&
          !browserSupported &&
          current !== info.serverLanguage
        ) {
          useLocaleStore.getState().setLocale(info.serverLanguage, "server");
          void i18n.changeLanguage(info.serverLanguage);
        }
      }
      if (location.pathname === "/setup") return;
      if (info.setupComplete === false) throw redirect({ to: "/setup" });
    } catch (err) {
      if (err && typeof err === "object" && "to" in err) throw err;
      console.warn("setup status fetch failed", err);
    }
  },
  component: RootComponent,
  errorComponent: RouteErrorComponent,
});

// PageViewReporter logs each in-app navigation to the audit log (one POST per
// distinct path, authenticated users only), so admins can see which pages
// users visit. Fire-and-forget; failures are swallowed.
function PageViewReporter() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const lastRef = useRef<string>("");
  useEffect(() => {
    if (!useAuthStore.getState().accessToken) return;
    if (!pathname || pathname === lastRef.current || pathname === "/login" || pathname === "/setup") return;
    lastRef.current = pathname;
    void activityServiceRecordPageView({ path: pathname }).catch(() => {});
  }, [pathname]);
  return null;
}

// DocumentTitle brands the tab with the admin's server name ("Salon", or the
// default "Evey"), read from the same setup-status query beforeLoad primed so
// it costs no extra request. Dev keeps its "[DEV]" marker (main.tsx).
function DocumentTitle() {
  const { data } = useQuery<{ serverName?: string }>({
    queryKey: ["setup-status"],
    queryFn: () => adminSystemServiceGetSetupStatus(),
    staleTime: 60_000,
  });
  useEffect(() => {
    const name = data?.serverName?.trim();
    if (!name) return;
    document.title = import.meta.env.DEV ? `${name} [DEV]` : name;
  }, [data?.serverName]);
  return null;
}

function RootComponent() {
  return (
    <ErrorBoundary>
      <ThemeProvider>
        <DocumentTitle />
        <PageViewReporter />
        <Outlet />
        <ToastContainer />
      </ThemeProvider>
    </ErrorBoundary>
  );
}
