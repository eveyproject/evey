import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  esbuild: {
    jsx: "automatic",
  },
  resolve: {
    alias: {
      "@evey/api": resolve(__dirname, "../../packages/api/src/fetcher.ts"),
      "react-native": resolve(__dirname, "./src/test/__mocks__/react-native.ts"),
      "expo-localization": resolve(__dirname, "./src/test/__mocks__/expo-localization.ts"),
      "expo-network": resolve(__dirname, "./src/test/__mocks__/expo-network.ts"),
    },
  },
  define: {
    __DEV__: true,
  },
  test: {
    globals: true,
    environment: "jsdom",
    css: false,
  },
});
