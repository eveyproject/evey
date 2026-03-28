import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { type QueryClient } from "@tanstack/react-query";
import { ThemeProvider } from "@/components/ThemeProvider";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { RouteErrorComponent } from "@/components/ui/RouteErrorComponent";
import { ToastContainer } from "@/components/ui/ToastContainer";
import { PersistentPlayer } from "@/features/playback/PersistentPlayer";

interface RouterContext {
  queryClient: QueryClient;
}

export const Route = createRootRouteWithContext<RouterContext>()({
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
