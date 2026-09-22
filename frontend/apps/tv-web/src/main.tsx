import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useAuthStore, useToastStore, shouldRetryQuery } from "@evey/store";
import { mutationErrorNet } from "@evey/client";
import { initTvNavigation, useTvKeys } from "./tv/focus";
import { useRoute, replace, goBack, type Route } from "./app/router";
import { bootSession, authRefresh, activeServerUrl } from "./lib/session";
import i18n from "./i18n";
import { ServerConnectScreen } from "./screens/ServerConnect";
import { LoginScreen } from "./screens/Login";
import { QuickConnectScreen } from "./screens/QuickConnect";
import { ProfileSelectScreen } from "./screens/ProfileSelect";
import { HomeScreen } from "./screens/Home";
import { LibraryScreen } from "./screens/Library";
import { MediaDetailScreen } from "./screens/MediaDetail";
import { PlayerScreen } from "./screens/Player";
import { SettingsScreen } from "./screens/Settings";
import { SearchScreen } from "./screens/Search";
import { applyThemeVars, useAppliedTheme } from "./lib/theme";
import "./styles.css";

// The default palette before anything loads; the profile's own theme
// follows from useAppliedTheme once the session is up.
applyThemeVars("default");

initTvNavigation();

const queryClient = new QueryClient({
  mutationCache: mutationErrorNet(),
  defaultOptions: {
    queries: { staleTime: 5 * 60 * 1000, retry: shouldRetryQuery },
  },
});

const PUBLIC_ROUTES = new Set<Route["name"]>(["server", "login", "quickconnect"]);

function Toasts() {
  const toasts = useToastStore((s) => s.toasts);
  const last = toasts[toasts.length - 1];
  if (!last) return null;
  return <div className="toast">{last.message}</div>;
}

// Paints the profile's theme; mounted only once signed in so its queries
// never fire before a server is chosen.
function ThemeFollower() {
  useAppliedTheme();
  return null;
}

function App() {
  const route = useRoute();
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    void (async () => {
      const { hasServer } = await bootSession(() => {
        if (!useAuthStore.getState().accessToken) return;
        useAuthStore.getState().logout();
        replace({ name: "login" });
      });
      if (hasServer) {
        // A refresh token from the last visit restores the session without a
        // password; "unavailable" (offline) keeps whatever the store held.
        await authRefresh.ensureAccessToken();
      }
      setReady(true);
    })();
  }, []);

  // The global back: a screen that handles back itself (the player) stops
  // the event before it reaches here.
  useTvKeys((key) => {
    if (key !== "back") return;
    if (route.name === "home" || route.name === "server" || route.name === "login") return;
    goBack();
  });

  useEffect(() => {
    if (!ready) return;
    if (!activeServerUrl()) {
      if (route.name !== "server") replace({ name: "server" });
      return;
    }
    if (!accessToken && !PUBLIC_ROUTES.has(route.name)) replace({ name: "login" });
    if (accessToken && (route.name === "login" || route.name === "quickconnect" || route.name === "server")) {
      replace(profileId ? { name: "home" } : { name: "profiles" });
    }
  }, [ready, route.name, accessToken, profileId]);

  useEffect(() => {
    document.documentElement.lang = i18n.language;
  }, []);

  if (!ready) return null;

  // The screen is decided here, synchronously, and the hash follows in the
  // effect above: rendering the route first and redirecting after let the
  // home mount for one frame with no server chosen and fire its queries at
  // nowhere.
  const effective: Route = !activeServerUrl()
    ? { name: "server" }
    : !accessToken && !PUBLIC_ROUTES.has(route.name)
      ? { name: "login" }
      : accessToken && PUBLIC_ROUTES.has(route.name)
        ? { name: profileId ? "home" : "profiles" }
        : route;

  const screen = renderScreen(effective);
  return (
    <>
      {accessToken && profileId ? <ThemeFollower /> : null}
      {screen}
    </>
  );
}

function renderScreen(effective: Route) {
  switch (effective.name) {
    case "server":
      return <ServerConnectScreen />;
    case "login":
      return <LoginScreen />;
    case "quickconnect":
      return <QuickConnectScreen />;
    case "profiles":
      return <ProfileSelectScreen />;
    case "library":
      return <LibraryScreen libraryId={effective.libraryId} />;
    case "media":
      return <MediaDetailScreen mediaId={effective.mediaId} />;
    case "player":
      return <PlayerScreen mediaId={effective.mediaId} restart={!!effective.restart} />;
    case "settings":
      return <SettingsScreen />;
    case "search":
      return <SearchScreen initialQuery={effective.query} />;
    default:
      return <HomeScreen />;
  }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
      <Toasts />
    </QueryClientProvider>
  </StrictMode>,
);
