import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { routeTree } from "./routeTree.gen";
import { useAuthStore, toast, type PersistedAuthState } from "@evey/store";
import { setTokenProvider, setTokensUpdater, setLocaleProvider, resetRefreshFailure, userServiceRefreshToken } from "@evey/api";
import { useLocaleStore } from "@evey/store";
import { registerServiceWorker } from "@/lib/webpush";
import { setupOnlineManager } from "@/lib/online-manager";
import { mutationErrorNet } from "@/lib/mutationErrorNet";
import { shouldRetryQuery } from "@/lib/queryRetry";
import { ThemeLoader } from "@/features/themes/ThemeLoader";
import "./i18n";
import "./styles/main.css";

registerServiceWorker();
setupOnlineManager();

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
  // a 401 just means the page made an unnecessary auth call - don't clear
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

// Proactive token refresh - refresh a bit before expiry to avoid 401 retries.
// Jitter + Web Locks serialize refreshes across tabs so two timers firing at
// the same instant don't both hit /auth/refresh with the same (in-flight-
// rotated) cookie, which the server would treat as reuse and purge all of
// the user's refresh tokens.
// How long before expiry the proactive refresh fires. Consistent with the
// fetcher's own EXPIRY_GRACE_SECONDS (a much smaller last-resort margin).
const REFRESH_MARGIN_MS = 2 * 60 * 1000;
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
    // While impersonating, the HttpOnly refresh cookie still belongs to the
    // admin (ImpersonateUser issues only an access token, no cookie rotation).
    // A proactive refresh would therefore silently mint a fresh ADMIN token and
    // revert identity mid-session - the banner vanishes but the UI stays on the
    // impersonated user's pages, so further actions run as the admin unannounced.
    // Instead of refreshing, end impersonation cleanly when the token expires,
    // which restores the admin's own token and leaves a consistent state.
    if (useAuthStore.getState().impersonatedBy) {
      refreshTimer = setTimeout(() => {
        if (!useAuthStore.getState().impersonatedBy) return;
        useAuthStore.getState().exitImpersonation();
        toast.info(i18n.t("admin.users.impersonationEnded"));
      }, Math.max(expiresMs - Date.now(), 0));
      return;
    }
    // Random jitter in [0, 30s] so two tabs don't wake at the same millisecond.
    const jitter = Math.floor(Math.random() * 30_000);
    const refreshAt = expiresMs - REFRESH_MARGIN_MS + jitter;
    const delay = Math.max(refreshAt - Date.now(), 0);
    refreshTimer = setTimeout(async () => {
      try {
        await runUnderTabLock(async () => {
          // Another tab may have refreshed while we waited on the lock - if
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

// Threshold below which a token is treated as about to expire (skip reusing
// it and refresh instead). Matches the jitter ceiling in scheduleTokenRefresh.
const NEAR_EXPIRY_MS = 60_000;
function isNearExpiry(token: string): boolean {
  try {
    const tokenParts = token.split(".");
    if (tokenParts.length < 3 || !tokenParts[1]) return true;
    const payload = JSON.parse(atob(tokenParts[1].replace(/-/g, "+").replace(/_/g, "/")));
    const msLeft = payload.exp * 1000 - Date.now();
    return msLeft < NEAR_EXPIRY_MS;
  } catch { return true; }
}

// Cross-tab sync: when Zustand persist writes to localStorage from another
// tab, our in-memory store is stale. Rehydrate on storage events so both
// tabs see the newly-refreshed access token.
if (typeof window !== "undefined") {
  window.addEventListener("storage", (ev) => {
    if (ev.key !== "evey-auth") return;
    const current = useAuthStore.getState().accessToken;
    // Key removed entirely (some browsers on clear) → treat as logout.
    if (!ev.newValue) {
      if (current) window.dispatchEvent(new Event("evey:unauthorized"));
      return;
    }
    try {
      const next = JSON.parse(ev.newValue) as { state?: PersistedAuthState };
      const persisted = next.state;
      if (!persisted) return;
      const newAccess = persisted.accessToken ?? null;
      // Logout in another tab: the persisted blob now has a null token. Clearing
      // only on token *change* (the old guard) left this tab authenticated -
      // a residual session on a shared machine. Reuse the canonical logout flow.
      if (!newAccess) {
        if (current) window.dispatchEvent(new Event("evey:unauthorized"));
        return;
      }
      // Token or impersonation changed in another tab → rehydrate the FULL blob.
      // setTokens alone dropped impersonationOrigin, so a cross-tab
      // exitImpersonation would no-op; hydrateFromPersisted restores it.
      const st = useAuthStore.getState();
      if (
        newAccess !== current ||
        persisted.impersonatedBy !== st.impersonatedBy ||
        persisted.impersonationOrigin !== st.impersonationOrigin
      ) {
        st.hydrateFromPersisted(persisted);
      }
    } catch { /* ignore malformed payload */ }
  });
}

useAuthStore.subscribe((state, prev) => {
  if (state.accessToken !== prev.accessToken) scheduleTokenRefresh();
  // A new token from OUTSIDE the fetcher (login, impersonation) starts a new
  // session: the fetcher's refresh-failure latch must not survive into it.
  if (state.accessToken && state.accessToken !== prev.accessToken) resetRefreshFailure();
});
scheduleTokenRefresh();

// Dev-only niceties. `import.meta.env.DEV` is true under `vite dev` and false
// in prod builds (`vite build`), so none of this leaks to pre-prod/prod.
if (import.meta.env.DEV) {
  // Mark the browser tab so a local dev session is never mistaken for prod.
  document.title = "Evey [DEV]";
  // `clearI18n()` in the console flushes cached translations.
  (window as never as Record<string, unknown>).clearI18n = () => {
    Object.keys(localStorage).filter((k) => k.startsWith("evey_i18n_")).forEach((k) => localStorage.removeItem(k));
    location.reload();
  };
}

const queryClient = queryClientRef = new QueryClient({
  mutationCache: mutationErrorNet(),
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      retry: shouldRetryQuery,
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
        // --filter-active-text is the on-accent foreground token: hardcoding
        // #fff here made the only escape hatch from a crashed app unreadable on
        // every theme whose accent is light (default, amoled, dark-red).
        style={{ padding: "8px 16px", background: "var(--accent, #1d4ed8)", color: "var(--filter-active-text, #fff)", border: "none", borderRadius: 4, cursor: "pointer" }}
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
