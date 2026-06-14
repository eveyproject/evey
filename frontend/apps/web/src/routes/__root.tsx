import { createRootRouteWithContext, Outlet, redirect } from "@tanstack/react-router";
import { type QueryClient } from "@tanstack/react-query";
import { adminSystemServiceGetSetupStatus } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import i18n from "@/i18n";
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
        const current = useLocaleStore.getState().locale;
        if (current !== info.serverLanguage) {
          useLocaleStore.getState().setLocale(info.serverLanguage);
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

function RootComponent() {
  return (
    <ErrorBoundary>
      <ThemeProvider>
        <Outlet />
        <ToastContainer />
      </ThemeProvider>
    </ErrorBoundary>
  );
}
