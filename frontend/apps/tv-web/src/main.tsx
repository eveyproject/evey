import { Component, StrictMode, useEffect, useState, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useAuthStore, shouldRetryQuery } from "@evey/store";
import { mutationErrorNet } from "@evey/client";
import { initTvNavigation, useTvKeys } from "./tv/focus";
import { useRoute, navigate, replace, goBack, type Route } from "./app/router";
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
import { MusicScreen, AlbumScreen, ArtistScreen } from "./screens/Music";
import {
  CollectionScreen,
  CollectionsScreen,
  FavoritesScreen,
  GenreScreen,
  GenresScreen,
  HistoryScreen,
  LibrariesScreen,
  NotificationsScreen,
  PersonScreen,
  PlaylistScreen,
  PlaylistsScreen,
} from "./screens/Browse";
import { PlaybackSettingsScreen, ServersScreen, StatsScreen } from "./screens/SettingsScreens";
import { PhotosScreen, SlideshowScreen } from "./screens/Photos";
import { PluginSettingsScreen } from "./screens/PluginSettings";
import { RequestsScreen } from "./screens/Requests";
import { SyncplayScreen } from "./screens/Syncplay";
import { TabShell, HomeShortcut, TAB_ROUTES } from "./components/Shell";
import { MusicBar } from "./music/MusicBar";
import { Toasts } from "./components/Toasts";
import { useMusicQueueStore } from "./music/queue";
import { applyThemeVars, useAppliedTheme } from "./lib/theme";
import { applyUiScale, storedUiScale } from "./lib/uiScale";
import { readChoice, shouldAskWhichClient, shouldGoToWebClient } from "./lib/clientChoice";
import { ClientChoice } from "./components/ClientChoice";
import { useDeviceRegistration, usePresenceHeartbeat } from "@evey/client/web";
import "./styles.css";

// A viewer who already answered "the web one" is sent there with no word;
// the question itself is asked on screen by <ClientChoice/>. There is no
// device test behind this - see lib/clientChoice.
if (shouldGoToWebClient({
  protocol: window.location.protocol,
  hostname: window.location.hostname,
  pathname: window.location.pathname,
  search: window.location.search,
  stored: readChoice(),
})) {
  // Says where the viewer came from, so the web client can offer a way back.
  window.location.replace("/?from=tv");
}

// The default palette before anything loads; the profile's own theme
// follows from useAppliedTheme once the session is up.
applyThemeVars("default");
// The interface size this television was set to, before the first paint.
applyUiScale(storedUiScale());

initTvNavigation();

const queryClient = new QueryClient({
  mutationCache: mutationErrorNet(),
  defaultOptions: {
    queries: { staleTime: 5 * 60 * 1000, retry: shouldRetryQuery },
  },
});

const PUBLIC_ROUTES = new Set<Route["name"]>(["server", "login", "quickconnect"]);

// A screen that throws must not take the whole television with it: the remote
// keeps a way back to the home.
class ScreenErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <div className="screen screen-center">
        <p className="error">{i18n.t("common.error")}</p>
        <button type="button" className="btn primary focusable" onClick={() => { this.setState({ failed: false }); replace({ name: "home" }); }}>
          {i18n.t("common.nav.home")}
        </button>
      </div>
    );
  }
}

// Paints the profile's theme; mounted only once signed in so its queries
// never fire before a server is chosen.
function ThemeFollower() {
  useAppliedTheme();
  return null;
}

// What the television announces about itself while someone is signed in: the
// presence beat for the admin's Online panel, and the Play On registration so
// a phone can send a title to this screen. The socket stays open: a
// television does not go to the background the way a phone does.
function DeviceServices() {
  usePresenceHeartbeat();
  useDeviceRegistration({
    deviceType: "tv",
    deviceName: () => "Evey TV (browser)",
    serverUrl: activeServerUrl(),
    pauseInBackground: false,
    onLoadAndPlay: (mediaId) => navigate({ name: "player", mediaId }),
  });
  return null;
}

function App() {
  const route = useRoute();
  // Asked before anything else is drawn: the question owns the screen until
  // it is answered (see ClientChoice).
  const [askClient, setAskClient] = useState(() => shouldAskWhichClient({
    protocol: window.location.protocol,
    hostname: window.location.hostname,
    pathname: window.location.pathname,
    search: window.location.search,
    stored: readChoice(),
  }));
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  // While an album plays its bar floats over the bottom edge: screens keep
  // that band clear and the home button rides above it.
  const musicOn = useMusicQueueStore((s) => s.currentIndex >= 0 && s.currentIndex < s.queue.length);
  useEffect(() => { document.body.classList.toggle("music-on", musicOn); }, [musicOn]);
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
    // The remote's search button opens the search from wherever the viewer
    // is, as the Android television's does.
    if (key === "search") {
      if (accessToken && profileId) navigate({ name: "search" });
      return;
    }
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

  if (askClient) return <ClientChoice onAnswered={() => setAskClient(false)} />;

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
  const signedIn = !!accessToken && !!profileId;
  // The Android television's layout: the tab screens sit in the rail and the
  // header; a deep screen is full width with a way home in its corner.
  const tab = signedIn && TAB_ROUTES.has(effective.name);
  const deep = signedIn && !tab && effective.name !== "player" && effective.name !== "slideshow" && effective.name !== "profiles";
  const guarded = <ScreenErrorBoundary key={effective.name}>{screen}</ScreenErrorBoundary>;
  return (
    <>
      {signedIn ? <ThemeFollower /> : null}
      {signedIn ? <DeviceServices /> : null}
      {tab ? <TabShell current={effective.name}>{guarded}</TabShell> : deep ? <div className="deep-screen">{guarded}</div> : guarded}
      {deep ? <HomeShortcut raised={musicOn} /> : null}
      {signedIn && effective.name !== "player" && effective.name !== "slideshow" ? <MusicBar /> : null}
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
      return <ProfileSelectScreen pick={!!effective.pick} />;
    case "library":
      return <LibraryScreen libraryId={effective.libraryId} />;
    case "media":
      return <MediaDetailScreen mediaId={effective.mediaId} />;
    case "player":
      return <PlayerScreen mediaId={effective.mediaId} restart={!!effective.restart} audio={effective.audio} subtitle={effective.subtitle} roomId={effective.roomId} />;
    case "settings":
      return <SettingsScreen />;
    case "search":
      return <SearchScreen initialQuery={effective.query} />;
    case "music":
      return <MusicScreen libraryId={effective.libraryId} />;
    case "album":
      return <AlbumScreen albumId={effective.albumId} libraryId={effective.libraryId} />;
    case "artist":
      return <ArtistScreen artistId={effective.artistId} libraryId={effective.libraryId} />;
    case "libraries":
      return <LibrariesScreen />;
    case "collections":
      return <CollectionsScreen />;
    case "collection":
      return <CollectionScreen collectionId={effective.collectionId} />;
    case "playlists":
      return <PlaylistsScreen />;
    case "playlist":
      return <PlaylistScreen playlistId={effective.playlistId} />;
    case "genres":
      return <GenresScreen />;
    case "genre":
      return <GenreScreen genre={effective.genre} />;
    case "favorites":
      return <FavoritesScreen />;
    case "history":
      return <HistoryScreen />;
    case "notifications":
      return <NotificationsScreen />;
    case "person":
      return <PersonScreen personId={effective.personId} />;
    case "playback":
      return <PlaybackSettingsScreen />;
    case "stats":
      return <StatsScreen />;
    case "servers":
      return <ServersScreen />;
    case "requests":
      return <RequestsScreen />;
    case "syncplay":
      return <SyncplayScreen />;
    case "photos":
      return <PhotosScreen libraryId={effective.libraryId} albumId={effective.albumId} />;
    case "plugin-settings":
      return <PluginSettingsScreen pluginId={effective.pluginId} />;
    case "slideshow":
      return <SlideshowScreen libraryId={effective.libraryId} albumId={effective.albumId} index={effective.index} />;
    default:
      return <HomeScreen customize={effective.name === "home" && !!effective.customize} />;
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
