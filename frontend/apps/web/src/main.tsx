import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { routeTree } from "./routeTree.gen";
import { useAuthStore } from "@evey/store";
import { setTokenProvider, setRefreshTokenProvider, setTokensUpdater } from "@evey/api";
import "./styles/main.css";

// Wire auth token into the API fetcher
setTokenProvider(() => useAuthStore.getState().accessToken);
setRefreshTokenProvider(() => useAuthStore.getState().refreshToken);
setTokensUpdater((access, refresh) => useAuthStore.getState().setTokens(access, refresh));

// Logout on 401
window.addEventListener("evey:unauthorized", () => useAuthStore.getState().logout());

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      retry: 2,
      refetchOnWindowFocus: false,
    },
  },
});

const router = createRouter({ routeTree, context: { queryClient } });

declare module "@tanstack/react-router" {
  interface Register { router: typeof router }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>
);
