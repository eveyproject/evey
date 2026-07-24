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
import { migrateFromSingleServer, useServerListStore } from "@evey/store";
import { configureAudioSession } from "../src/lib/audio-session";

// Wire Zustand persist to expo-secure-store (must run before stores are accessed)
setZustandStorage({
  getItem: (name) => SecureStore.getItemAsync(name),
  setItem: (name, value) => SecureStore.setItemAsync(name, value),
  removeItem: (name) => SecureStore.deleteItemAsync(name),
});

export default function RootLayout() {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    async function bootstrap() {
      // Migrate single-server users to the multi-server store (idempotent)
      try {
        await migrateFromSingleServer({ readLegacyServerUrl: getServerUrl });
      } catch (err) {
        console.error("server migration failed:", err);
      }
      await initFetcher();
      await configureAudioSession();
      const serverUrl =
        useServerListStore.getState().getActiveServer()?.url ?? await getServerUrl();
      if (serverUrl) {
        await initI18n(serverUrl);
        await applyServerLanguage(serverUrl);
      }
    }
    bootstrap()
      .catch((err) => console.error("bootstrap failed:", err))
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
