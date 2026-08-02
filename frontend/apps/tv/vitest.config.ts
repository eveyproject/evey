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
      "@evey/store": resolve(__dirname, "../../packages/store/src/index.ts"),
      "@evey/theme": resolve(__dirname, "../../packages/theme/src/index.ts"),
      "react-native": resolve(__dirname, "./src/test/__mocks__/react-native.ts"),
      "expo-secure-store": resolve(__dirname, "./src/test/__mocks__/expo-secure-store.ts"),
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
