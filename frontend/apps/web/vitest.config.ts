import { defineConfig } from "vitest/config";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [],
  resolve: {
    alias: [
      // Deep entries first: an alias matches by prefix, in order.
      { find: /^@evey\/client\/web\/(.+)$/, replacement: resolve(__dirname, "../../packages/client/src/web/$1") },
      { find: "@evey/client/web", replacement: resolve(__dirname, "../../packages/client/src/web.ts") },
      { find: "@", replacement: resolve(__dirname, "./src") },
      { find: "@evey/api", replacement: resolve(__dirname, "../../packages/api/src/index.ts") },
      { find: "@evey/client", replacement: resolve(__dirname, "../../packages/client/src/index.ts") },
      { find: "@evey/media-caps/cast", replacement: resolve(__dirname, "../../packages/media-caps/src/castUrl.ts") },
      { find: "@evey/media-caps/caps", replacement: resolve(__dirname, "../../packages/media-caps/src/castCaps.ts") },
      { find: "@evey/store", replacement: resolve(__dirname, "../../packages/store/src/index.ts") },
    ],
  },
  test: {
    globals: true,
    // happy-dom: the same suite ran in 33 s against jsdom's 52 s on one box
    // (environment 64 s vs 133 s of worker time, measured 2026-09-27); on the
    // 4-vCPU CI runner the jsdom suite was the frontend job's 18 minutes.
    environment: "happy-dom",
    setupFiles: ["./src/test/setup.ts"],
    // The admin settings page mounts every section in happy-dom: close to
    // five seconds on a two-core machine, so the default limit flaked under load.
    testTimeout: 15_000,
    css: false,
  },
});
