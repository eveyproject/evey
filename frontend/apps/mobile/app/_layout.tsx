import { useEffect, useState } from "react";
import { Stack } from "expo-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { SafeAreaProvider } from "react-native-safe-area-context";
import * as SecureStore from "expo-secure-store";
import { setZustandStorage } from "@evey/store";
import { queryClient } from "../src/lib/query-client";
import { ThemeProvider } from "../src/providers/ThemeProvider";
import { ToastContainer } from "../src/components/ToastContainer";
import { ErrorBoundary } from "../src/components/ErrorBoundary";
import { initFetcher } from "../src/lib/fetcher-setup";
import { getServerUrl } from "../src/lib/server-config";
import { initI18n } from "../src/i18n";
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
      await configureAudioSession();
      await initFetcher();
      const serverUrl = await getServerUrl();
      if (serverUrl) await initI18n(serverUrl);
    }
    bootstrap()
      .catch((err) => console.error("bootstrap failed:", err))
      .finally(() => setReady(true));
  }, []);

  if (!ready) return null;

  return (
    // @ts-expect-error React 19 dual @types/react versions
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <SafeAreaProvider>
          <ThemeProvider>
            <Stack screenOptions={{ headerShown: false }} />
            <ToastContainer />
          </ThemeProvider>
        </SafeAreaProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}
