import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  plugins: [],
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
      "@evey/api": resolve(__dirname, "../../packages/api/src/index.ts"),
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
