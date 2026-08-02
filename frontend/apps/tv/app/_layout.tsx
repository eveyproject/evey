import { useEffect, useState } from "react";
import { Stack } from "expo-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { SafeAreaProvider } from "react-native-safe-area-context";
import * as SecureStore from "expo-secure-store";
import { setZustandStorage } from "@evey/store";
import { queryClient } from "../src/lib/query-client";
import { ThemeProvider } from "../src/providers/ThemeProvider";
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
      // Migrate single-server users to the multi-server store (idempotent)
      try {
        await migrateFromSingleServer({ readLegacyServerUrl: getServerUrl });
      } catch (err) {
        bootLog.error("migration serveur échouée", err);
      }
      await initFetcher();
      await configureAudioSession();
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
  }, []);

  if (!ready) return null;

  return (
    <QueryClientProvider client={queryClient}>
      <SafeAreaProvider>
        <ThemeProvider>
          <ErrorBoundary>
            <Stack screenOptions={{ headerShown: false }} />
          </ErrorBoundary>
          <ToastContainer />
        </ThemeProvider>
      </SafeAreaProvider>
    </QueryClientProvider>
  );
}
