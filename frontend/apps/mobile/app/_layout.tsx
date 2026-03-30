import { useEffect, useState } from "react";
import { Stack } from "expo-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { queryClient } from "../src/lib/query-client";
import { ThemeProvider } from "../src/providers/ThemeProvider";
import { initFetcher } from "../src/lib/fetcher-setup";
import { getServerUrl } from "../src/lib/server-config";
import { initI18n } from "../src/i18n";

export default function RootLayout() {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    async function bootstrap() {
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
    <QueryClientProvider client={queryClient}>
      <SafeAreaProvider>
        <ThemeProvider>
          <Stack screenOptions={{ headerShown: false }} />
        </ThemeProvider>
      </SafeAreaProvider>
    </QueryClientProvider>
  );
}
