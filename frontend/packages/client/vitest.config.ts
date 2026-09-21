import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  esbuild: {
    jsx: "automatic",
  },
  resolve: {
    alias: {
      // Point at the package indexes, like the apps do, so the shared code
      // under test is the one that ships and not a hand-written stub.
      "@evey/api": resolve(__dirname, "../api/src/index.ts"),
      "@evey/store": resolve(__dirname, "../store/src/index.ts"),
      "@evey/media-caps": resolve(__dirname, "../media-caps/src/index.ts"),
      // The native entry imports react-native and expo-secure-store; neither
      // loads under vitest, so the same doubles the apps use stand in here.
      "react-native": resolve(__dirname, "./src/test/__mocks__/react-native.ts"),
      "expo-secure-store": resolve(__dirname, "./src/test/__mocks__/expo-secure-store.ts"),
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    css: false,
  },
});
