import { useEffect, useState } from "react";
import { AppState, StatusBar, View } from "react-native";
import { Stack } from "expo-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { SafeAreaProvider } from "react-native-safe-area-context";
import * as SecureStore from "expo-secure-store";
import { setZustandStorage } from "@evey/store";
import { queryClient } from "../src/lib/query-client";
import { ThemeProvider, useTheme } from "../src/providers/ThemeProvider";
import { ToastContainer } from "../src/components/ToastContainer";
import { ErrorBoundary } from "../src/components/ErrorBoundary";
import { initFetcher } from "../src/lib/fetcher-setup";
import { getServerUrl, resolveServerUrl } from "../src/lib/server-config";
import { migrateFromSingleServer, useAuthStore, useServerListStore } from "@evey/store";
import { persistQueryCache, queryCacheScope, restoreQueryCache } from "@evey/client";
import { fileQueryStorage } from "@evey/client/native/queryPersistStorage";
import { applyServerLanguage, initI18n, storeHydrated } from "../src/i18n";
import { configureAudioSession } from "../src/lib/audio-session";
import { refreshAccessTokenIfNeeded } from "../src/lib/auth-refresh";
import { purgeOrphanedDownloads, rehydrateBackgroundDownloads } from "../src/lib/downloads";

// Wire Zustand persist to expo-secure-store (must run before stores are accessed)
setZustandStorage({
  getItem: (name) => SecureStore.getItemAsync(name),
  setItem: (name, value) => SecureStore.setItemAsync(name, value),
  removeItem: (name) => SecureStore.deleteItemAsync(name),
});

// Ceiling on any single network step of the boot sequence. Offline is a
// supported way to start this app (a downloaded title must play on a plane),
// so no step of the splash may wait on an answer that is not coming.
const BOOT_NETWORK_BUDGET_MS = 6000;

const currentQueryScope = () =>
  queryCacheScope(useServerListStore.getState().activeServerId, useAuthStore.getState().profileId);

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export default function RootLayout() {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    async function bootstrap() {
      await configureAudioSession();
      // Migrate single-server users to the multi-server store (idempotent)
      try {
        await migrateFromSingleServer({ readLegacyServerUrl: getServerUrl });
      } catch (err) {
        console.error("server migration failed:", err);
      }
      await initFetcher();
      // Wait for the server-list store to rehydrate BEFORE resolving: an
      // install without the legacy SecureStore url (any multi-server-era
      // install) otherwise resolves null here, i18n initialises against a
      // dead relative loadPath, and the whole session renders raw keys while
      // media - whose fetches read the store lazily, post-hydration - loads
      // fine. Same family as applyServerLanguage's own hydration wait.
      await storeHydrated(useServerListStore);
      const serverUrl = await resolveServerUrl(useServerListStore.getState().getActiveServer()?.url);
      // ALWAYS initialise i18n - even with no server yet. Without this the
      // first-launch screens (server-connect, then login) rendered raw keys
      // until the app was restarted (2026-07-29). With no server the embedded
      // English floor answers; the instance catalogue merges over it as soon
      // as a server is set.
      // Bounded: the catalogue is fetched over HTTP with no timeout of its
      // own, and nothing renders until this resolves. A server that refuses
      // the connection fails fast, but one that ACCEPTS and never answers
      // (sleeping NAS on a reachable LAN, captive portal) holds the splash for
      // the OS connect timeout. The embedded English floor is already in
      // memory by then, and the (auth) layout retries initI18n on mount, so
      // giving up here costs a few seconds of English at worst.
      await Promise.race([initI18n(serverUrl ?? ""), delay(BOOT_NETWORK_BUDGET_MS)]);
      if (serverUrl) {
        // Same budget, same reason: its last resort is a /setup/status fetch.
        await Promise.race([applyServerLanguage(serverUrl), delay(BOOT_NETWORK_BUDGET_MS)]);
        // Silently refresh a stale access token on cold start so the user
        // is not bounced to /login when the cached JWT has expired.
        await refreshAccessTokenIfNeeded();
        // The last session's first screens, shown while they refresh: a cold
        // start no longer waits for the home's round trips.
        await restoreQueryCache(queryClient, fileQueryStorage, currentQueryScope());
      }
      // Re-attach JS callbacks to any background downloads that survived an
      // app kill - the native foreground service kept them alive, but JS
      // has no memory of them after a fresh process start. Without this,
      // the index never gets updated when those downloads finish.
      //
      // Fire-and-forget: this only re-wires existing tasks (it doesn't start
      // anything new) and we don't want to block the splash screen on the
      // native module's lookup. The orphan sweep chains AFTER rehydration on
      // purpose: a rehydrated in-flight task's partial file is on disk but
      // not in the index, and sweeping first would delete it mid-download.
      // The audio-pass sweep chains LAST: it walks gigabytes when a pass was
      // interrupted, and it must see the index as the orphan sweep left it.
      void rehydrateBackgroundDownloads()
        .then(() => purgeOrphanedDownloads())
        .then(() => import("../src/lib/audioCompatPass"))
        .then((m) => m.resumePendingAudioPasses())
        .catch((err) => console.warn("evey-audiopass sweep", err));
    }
    const stopPersisting = persistQueryCache(queryClient, fileQueryStorage, currentQueryScope);
    bootstrap()
      .catch((err) => console.error("bootstrap failed:", err))
      .finally(() => setReady(true));

    const sub = AppState.addEventListener("change", (next) => {
      if (next === "active") {
        void refreshAccessTokenIfNeeded();
      }
    });
    return () => {
      sub.remove();
      stopPersisting();
    };
  }, []);

  if (!ready) return null;

  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <SafeAreaProvider>
          <ThemeProvider>
            <RootStack />
            <ToastContainer />
          </ThemeProvider>
        </SafeAreaProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

// Native Stack headers already account for the status bar / camera notch
// inset on their own, so this wrapper must NOT add a paddingTop here - doing
// so used to double the top margin on every screen with a header. Screens
// without a native header are responsible for their own inset.
function RootStack() {
  const theme = useTheme();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <StatusBar barStyle={isLightBg(theme.colors.bg) ? "dark-content" : "light-content"} />
      <Stack screenOptions={{ headerShown: false }} />
    </View>
  );
}

function isLightBg(hex: string): boolean {
  const m = hex.match(/^#?([0-9a-f]{6})$/i);
  if (!m) return false;
  const v = parseInt(m[1], 16);
  const r = (v >> 16) & 0xff;
  const g = (v >> 8) & 0xff;
  const b = v & 0xff;
  // Perceived luminance (BT.601). Above ~140 is "light enough" that white
  // status-bar icons would wash out.
  return r * 0.299 + g * 0.587 + b * 0.114 > 140;
}
