import { defineConfig } from "vitest/config";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [],
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
      "@evey/api": resolve(__dirname, "../../packages/api/src/index.ts"),
      "@evey/media-caps/cast": resolve(__dirname, "../../packages/media-caps/src/castUrl.ts"),
      "@evey/media-caps/caps": resolve(__dirname, "../../packages/media-caps/src/castCaps.ts"),
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
