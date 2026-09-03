import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  esbuild: {
    jsx: "automatic",
  },
  resolve: {
    alias: {
      // Point at the package INDEX, like web does. Aliasing straight to fetcher.ts
      // made every shared export (library rules, pagination helpers) invisible to
      // these apps' tests, so a screen test could only ever run against
      // hand-written stubs of code that ships.
      "@evey/api": resolve(__dirname, "../../packages/api/src/index.ts"),
      "react-native": resolve(__dirname, "./src/test/__mocks__/react-native.ts"),
      "expo-localization": resolve(__dirname, "./src/test/__mocks__/expo-localization.ts"),
      "expo-network": resolve(__dirname, "./src/test/__mocks__/expo-network.ts"),
      "react-native-webview": resolve(__dirname, "./src/test/__mocks__/react-native-webview.ts"),
      "expo-file-system/legacy": resolve(__dirname, "./src/test/__mocks__/expo-file-system-legacy.ts"),
      "expo-secure-store": resolve(__dirname, "./src/test/__mocks__/expo-secure-store.ts"),
      // Each icon family is imported by its own path so only the fonts in use
      // ship; vite cannot parse the real modules, so they all resolve here.
      "@expo/vector-icons/Ionicons": resolve(__dirname, "./src/test/__mocks__/vector-icon.ts"),
      "@expo/vector-icons/MaterialIcons": resolve(__dirname, "./src/test/__mocks__/vector-icon.ts"),
    },
  },
  define: {
    __DEV__: true,
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
