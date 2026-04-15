import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { routeTree } from "./routeTree.gen";
import { useAuthStore } from "@evey/store";
import { setTokenProvider, setTokensUpdater, setLocaleProvider, userServiceRefreshToken } from "@evey/api";
import { useLocaleStore } from "@evey/store";
import { registerServiceWorker } from "@/lib/webpush";
import "./i18n";
import "./styles/main.css";

registerServiceWorker();

// Wire auth token into the API fetcher
setTokenProvider(() => useAuthStore.getState().accessToken);
setTokensUpdater((access) => useAuthStore.getState().setTokens(access));
setLocaleProvider(() => useLocaleStore.getState().locale);

// Sync locale store changes to i18next
import i18n from "./i18n";
useLocaleStore.subscribe((state) => {
  if (state.locale && i18n.language !== state.locale) {
    void i18n.changeLanguage(state.locale);
  }
});

// Logout on 401 and redirect to login (queryClient cleared lazily — defined below)
window.addEventListener("evey:unauthorized", () => {
  // Guard against feedback loop on already-logged-out pages: if no token,
  // a 401 just means the page made an unnecessary auth call — don't clear
  // queries or redirect, that triggers another mount → query → 401 → loop.
  if (!useAuthStore.getState().accessToken) return;
  queryClientRef?.clear();
  useAuthStore.getState().logout();
  localStorage.removeItem("evey-recent-searches");
  if (window.location.pathname !== "/login") {
    window.location.href = "/login";
  }
});
let queryClientRef: QueryClient | null = null;

// Proactive token refresh — refresh 2 min before expiry to avoid 401 retries
let refreshTimer: ReturnType<typeof setTimeout> | null = null;
function scheduleTokenRefresh() {
  if (refreshTimer) { clearTimeout(refreshTimer); refreshTimer = null; }
  const token = useAuthStore.getState().accessToken;
  if (!token) return;
  try {
    const payload = JSON.parse(atob(token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/")));
    const expiresMs = payload.exp * 1000;
    const refreshAt = expiresMs - 2 * 60 * 1000; // 2 min before expiry
    const delay = Math.max(refreshAt - Date.now(), 0);
    refreshTimer = setTimeout(async () => {
      try {
        const data = await userServiceRefreshToken({});
        if (data?.accessToken) {
          useAuthStore.getState().setTokens(data.accessToken);
        }
      } catch (err) { console.warn("token refresh failed", err); }
      scheduleTokenRefresh();
    }, delay);
  } catch (err) { console.warn("parse JWT failed", err); }
}
useAuthStore.subscribe((state, prev) => {
  if (state.accessToken !== prev.accessToken) scheduleTokenRefresh();
});
scheduleTokenRefresh();

// Dev helper: `clearI18n()` in console to flush cached translations
if (import.meta.env.DEV) {
  (window as never as Record<string, unknown>).clearI18n = () => {
    Object.keys(localStorage).filter((k) => k.startsWith("evey_i18n_")).forEach((k) => localStorage.removeItem(k));
    location.reload();
  };
}

const queryClient = queryClientRef = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      retry: (count, error) => {
        if ((error as { httpStatus?: number }).httpStatus === 401) return false;
        return count < 2;
      },
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
