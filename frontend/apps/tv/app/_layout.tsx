import { useEffect, useState } from "react";
import { Stack } from "expo-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { AppState } from "react-native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import * as SecureStore from "expo-secure-store";
import { setZustandStorage } from "@evey/store";
import { queryClient } from "../src/lib/query-client";
import { ThemeProvider, useTheme } from "../src/providers/ThemeProvider";
import { ErrorBoundary } from "../src/components/ErrorBoundary";
import { ToastContainer } from "../src/components/ToastContainer";
import { initFetcher } from "../src/lib/fetcher-setup";
import { getServerUrl } from "../src/lib/server-config";
import { applyServerLanguage, initI18n } from "../src/i18n";
import { refreshAccessTokenIfNeeded } from "../src/lib/auth-refresh";
import { migrateFromSingleServer, useServerListStore } from "@evey/store";
import { configureAudioSession } from "../src/lib/audio-session";
import { installCrashGuard } from "../src/lib/crashGuard";
import { bootMarker, logger } from "../src/lib/log";



// Wire Zustand persist to expo-secure-store (must run before stores are accessed)
setZustandStorage({
  getItem: (name) => SecureStore.getItemAsync(name),
  setItem: (name, value) => SecureStore.setItemAsync(name, value),
  removeItem: (name) => SecureStore.deleteItemAsync(name),
});

const bootLog = logger("boot");

// The root card carries the theme so no wrapper has to paint it again under
// every screen (see the (auth) layout for the overdraw measurement).
function RootStack() {
  const theme = useTheme();
  return <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: theme.colors.bg } }} />;
}

export default function RootLayout() {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    // Installed here and NOT at module scope. Writing to the console during
    // bundle evaluation - before the bridgeless runtime has wired it up -
    // segfaults the JS thread about 100 ms after boot, every launch, with no
    // JavaScript-level trace. Cost an hour to bisect; the guard that was meant
    // to stop crashes was causing one.
    installCrashGuard();
    bootMarker();

    async function bootstrap() {
      // Migrate single-server users to the multi-server store (idempotent).
      // The audio session has no dependency on the store migration or the
      // fetcher: running the three serially just lengthened the black screen
      // before the home (every await here is time with nothing painted).
      await Promise.all([
        migrateFromSingleServer({ readLegacyServerUrl: getServerUrl }).catch((err) => {
          bootLog.error("migration serveur échouée", err);
        }),
        configureAudioSession(),
      ]);
      await initFetcher();
      const serverUrl =
        useServerListStore.getState().getActiveServer()?.url ?? await getServerUrl();
      if (serverUrl) {
        await initI18n(serverUrl);
        await applyServerLanguage(serverUrl);
        // Renew a stale access token before the first screen decides where to
        // send the viewer: without this a cold start lands on /login, and a TV
        // has no keyboard to type a password with.
        await refreshAccessTokenIfNeeded();
      }
    }
    bootstrap()
      // Never block the app on a failed bootstrap: i18n or the audio session
      // failing must leave a usable screen, not a permanent null render.
      .catch((err) => bootLog.error("bootstrap échoué", err))
      .finally(() => setReady(true));

    // Android TV suspends the app when the set switches input or goes to
    // standby - hours later the access token is long expired. Renew it as the
    // app comes back to the foreground so the first visible queries carry a
    // live token instead of each tripping the 401-retry path at once.
    const sub = AppState.addEventListener("change", (next) => {
      if (next === "active") void refreshAccessTokenIfNeeded();
    });
    return () => sub.remove();
  }, []);

  if (!ready) return null;

  return (
    <QueryClientProvider client={queryClient}>
      <SafeAreaProvider>
        <ThemeProvider>
          <ErrorBoundary>
            <RootStack />
          </ErrorBoundary>
          <ToastContainer />
        </ThemeProvider>
      </SafeAreaProvider>
    </QueryClientProvider>
  );
}
