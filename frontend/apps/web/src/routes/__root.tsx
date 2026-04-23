import { createRootRouteWithContext, Outlet, redirect } from "@tanstack/react-router";
import { type QueryClient } from "@tanstack/react-query";
import { adminSystemServiceGetSetupStatus } from "@evey/api";
import { ThemeProvider } from "@/components/ThemeProvider";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";
import { ToastContainer } from "@/components/ui/ToastContainer";
import { PersistentPlayer } from "@/features/playback/PersistentPlayer";

interface RouterContext {
  queryClient: QueryClient;
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ context, location }) => {
    if (location.pathname === "/setup") return;
    try {
      const info = await context.queryClient.fetchQuery<{ setupComplete?: boolean }>({
        queryKey: ["setup-status"],
        queryFn: () => adminSystemServiceGetSetupStatus(),
        staleTime: 60_000,
      });
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
        <PersistentPlayer />
        <ToastContainer />
      </ThemeProvider>
    </ErrorBoundary>
  );
}
