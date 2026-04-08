import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  esbuild: {
    jsx: "automatic",
  },
  resolve: {
    alias: {
      "@evey/api": resolve(__dirname, "../../packages/api/src/fetcher.ts"),
      "@evey/store": resolve(__dirname, "../../packages/store/src/index.ts"),
      "@evey/theme": resolve(__dirname, "../../packages/theme/src/index.ts"),
      "react-native": resolve(__dirname, "./src/test/__mocks__/react-native.ts"),
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
