import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  esbuild: {
    jsx: "automatic",
  },
  resolve: {
    alias: [
      // Deep entries first: an alias matches by prefix, in order.
      { find: /^@evey\/client\/web\/(.+)$/, replacement: resolve(__dirname, "../../packages/client/src/web/$1") },
      { find: "@", replacement: resolve(__dirname, "./src") },
      { find: "@evey/api", replacement: resolve(__dirname, "../../packages/api/src/index.ts") },
      { find: "@evey/client/web", replacement: resolve(__dirname, "../../packages/client/src/web.ts") },
      { find: "@evey/client", replacement: resolve(__dirname, "../../packages/client/src/index.ts") },
      { find: "@evey/store", replacement: resolve(__dirname, "../../packages/store/src/index.ts") },
      { find: "@evey/theme", replacement: resolve(__dirname, "../../packages/theme/src/index.ts") },
    ],
  },
  test: {
    globals: true,
    environment: "jsdom",
    css: false,
  },
});
