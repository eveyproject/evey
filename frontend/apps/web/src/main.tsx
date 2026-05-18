import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { routeTree } from "./routeTree.gen";
import { useAuthStore } from "@evey/store";
import { setTokenProvider, setTokensUpdater, setLocaleProvider, userServiceRefreshToken } from "@evey/api";
import { useLocaleStore } from "@evey/store";
import { registerServiceWorker } from "@/lib/webpush";
import { ThemeLoader } from "@/features/themes/ThemeLoader";
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

// Logout on 401 and redirect to login (queryClient + router refs filled in below)
window.addEventListener("evey:unauthorized", () => {
  // Guard against feedback loop on already-logged-out pages: if no token,
  // a 401 just means the page made an unnecessary auth call — don't clear
  // queries or redirect, that triggers another mount → query → 401 → loop.
  if (!useAuthStore.getState().accessToken) return;
  queryClientRef?.clear();
  useAuthStore.getState().logout();
  localStorage.removeItem("evey-recent-searches");
  if (window.location.pathname !== "/login") {
    if (routerRef) {
      void routerRef.navigate({ to: "/login" });
    } else {
      window.location.href = "/login";
    }
  }
});
let queryClientRef: QueryClient | null = null;
let routerRef: ReturnType<typeof createRouter> | null = null;

// Proactive token refresh — refresh a bit before expiry to avoid 401 retries.
// Jitter + Web Locks serialize refreshes across tabs so two timers firing at
// the same instant don't both hit /auth/refresh with the same (in-flight-
// rotated) cookie, which the server would treat as reuse and purge all of
// the user's refresh tokens.
let refreshTimer: ReturnType<typeof setTimeout> | null = null;
function scheduleTokenRefresh() {
  if (refreshTimer) { clearTimeout(refreshTimer); refreshTimer = null; }
  const token = useAuthStore.getState().accessToken;
  if (!token) return;
  try {
    const tokenParts = token.split(".");
    if (tokenParts.length < 3 || !tokenParts[1]) return;
    const payload = JSON.parse(atob(tokenParts[1].replace(/-/g, "+").replace(/_/g, "/")));
    const expiresMs = payload.exp * 1000;
    // Random jitter in [0, 30s] so two tabs don't wake at the same millisecond.
    const jitter = Math.floor(Math.random() * 30_000);
    const refreshAt = expiresMs - 2 * 60 * 1000 + jitter;
    const delay = Math.max(refreshAt - Date.now(), 0);
    refreshTimer = setTimeout(async () => {
      try {
        await runUnderTabLock(async () => {
          // Another tab may have refreshed while we waited on the lock — if
          // our access token has been replaced and is not about to expire, skip.
          const current = useAuthStore.getState().accessToken;
          if (current && current !== token && !isNearExpiry(current)) return;
          const data = await userServiceRefreshToken({});
          if (data?.accessToken) {
            useAuthStore.getState().setTokens(data.accessToken);
          }
        });
      } catch (err) { console.warn("token refresh failed", err); }
      scheduleTokenRefresh();
    }, delay);
  } catch (err) { console.warn("parse JWT failed", err); }
}

async function runUnderTabLock(fn: () => Promise<void>): Promise<void> {
  if (typeof navigator !== "undefined" && "locks" in navigator) {
    await (navigator as Navigator & { locks: LockManager }).locks.request(
      "evey-refresh-token",
      { mode: "exclusive" },
      fn,
    );
    return;
  }
  await fn();
}

function isNearExpiry(token: string): boolean {
  try {
    const tokenParts = token.split(".");
    if (tokenParts.length < 3 || !tokenParts[1]) return true;
    const payload = JSON.parse(atob(tokenParts[1].replace(/-/g, "+").replace(/_/g, "/")));
    const msLeft = payload.exp * 1000 - Date.now();
    return msLeft < 60_000;
  } catch { return true; }
}

// Cross-tab sync: when Zustand persist writes to localStorage from another
// tab, our in-memory store is stale. Rehydrate on storage events so both
// tabs see the newly-refreshed access token.
if (typeof window !== "undefined") {
  window.addEventListener("storage", (ev) => {
    if (ev.key !== "evey-auth" || !ev.newValue) return;
    try {
      const next = JSON.parse(ev.newValue) as { state?: { accessToken?: string | null } };
      const newAccess = next.state?.accessToken ?? null;
      const current = useAuthStore.getState().accessToken;
      if (newAccess && newAccess !== current) {
        useAuthStore.getState().setTokens(newAccess);
      }
    } catch { /* ignore malformed payload */ }
  });
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

// Fallback shown when a route render throws. Keeps the rest of the app
// functional and gives the user an escape hatch (reload) instead of a blank
// white page. The inner component receives `error` + `reset` via props; we
// render minimal, dependency-free HTML so even if the theme CSS fails, the
// message is legible.
function RouterErrorFallback({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div role="alert" style={{ padding: 24, color: "var(--text, #eee)", background: "var(--bg, #111)" }}>
      <h1 style={{ fontSize: 20, marginBottom: 8 }}>Something went wrong</h1>
      <p style={{ fontSize: 14, opacity: 0.8, marginBottom: 16 }}>
        The app hit an unexpected error. Try reloading; if it persists, check the
        server logs or open an issue.
      </p>
      <pre style={{ fontSize: 12, opacity: 0.6, whiteSpace: "pre-wrap", marginBottom: 16 }}>
        {error.message}
      </pre>
      <button
        onClick={() => { reset(); window.location.reload(); }}
        style={{ padding: "8px 16px", background: "var(--accent, #3b82f6)", color: "#fff", border: "none", borderRadius: 4, cursor: "pointer" }}
      >
        Reload
      </button>
    </div>
  );
}

const router = routerRef = createRouter({
  routeTree,
  context: { queryClient },
  defaultErrorComponent: RouterErrorFallback,
});

declare module "@tanstack/react-router" {
  interface Register { router: typeof router }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeLoader />
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>
);
