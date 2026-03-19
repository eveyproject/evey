import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  plugins: [],
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
      // generated files don't exist in all worktrees; tests mock @evey/api anyway
      // more specific path first so it's not shadowed by prefix match
      "@evey/api/src/fetcher": resolve(__dirname, "../../packages/api/src/fetcher.ts"),
      "@evey/api/src/generated": resolve(__dirname, "../../packages/api/src/generated"),
      "@evey/api": resolve(__dirname, "../../packages/api/src/fetcher.ts"),
      "@evey/store": resolve(__dirname, "../../packages/store/src/index.ts"),
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
