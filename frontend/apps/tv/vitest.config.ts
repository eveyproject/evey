import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  esbuild: {
    jsx: "automatic",
  },
  resolve: {
    alias: [
      // Deep native modules first: an alias matches by prefix, in order.
      { find: /^@evey\/client\/native\/(.+)$/, replacement: resolve(__dirname, "../../packages/client/src/native/$1") },
      // Point at the package INDEX, like web does. Aliasing straight to fetcher.ts
      // made every shared export (library rules, pagination helpers) invisible to
      // these apps' tests, so a screen test could only ever run against
      // hand-written stubs of code that ships.
      { find: "@evey/api", replacement: resolve(__dirname, "../../packages/api/src/index.ts") },
      { find: "@evey/client/native", replacement: resolve(__dirname, "../../packages/client/src/native.ts") },
      { find: "@evey/client", replacement: resolve(__dirname, "../../packages/client/src/index.ts") },
      { find: "@evey/store", replacement: resolve(__dirname, "../../packages/store/src/index.ts") },
      { find: "@evey/theme", replacement: resolve(__dirname, "../../packages/theme/src/index.ts") },
      { find: "react-native", replacement: resolve(__dirname, "./src/test/__mocks__/react-native.ts") },
      { find: "expo-file-system/legacy", replacement: resolve(__dirname, "./src/test/__mocks__/expo-file-system-legacy.ts") },
      { find: "expo-secure-store", replacement: resolve(__dirname, "./src/test/__mocks__/expo-secure-store.ts") },
      // The shared plugin host (@evey/client/native/PluginWebView) imports the
      // WebView from the package, where a per-test vi.mock does not reach.
      { find: "react-native-webview", replacement: resolve(__dirname, "./src/test/__mocks__/react-native-webview.ts") },
      { find: "react-native-svg", replacement: resolve(__dirname, "./src/test/__mocks__/react-native-svg.ts") },
      { find: "@expo/vector-icons/Ionicons", replacement: resolve(__dirname, "./src/test/__mocks__/vector-icon.ts") },
    ],
  },
  define: {
    __DEV__: true,
  },
  test: {
    globals: true,
    environment: "happy-dom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
