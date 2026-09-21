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
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    css: false,
  },
});
