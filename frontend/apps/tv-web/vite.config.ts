import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "path";

// The television client. Three constraints shape this config:
//   - base "./": the same build is served by the Evey server under /tv AND
//     copied as is into a webOS .ipk / Tizen .wgt, where it runs from the
//     TV's storage (file://). Relative asset paths work in both.
//   - target chrome79: webOS 6.x (2021 sets) ships Chromium 79, and every
//     later webOS and Tizen runs something newer. Older sets get the
//     "browser too old" screen from the feature probe in index.html.
//   - hash routing (see src/app/router.tsx): no server-side rewrite is
//     needed under /tv, and file:// has no routes at all.
export default defineConfig({
  base: "./",
  plugins: [react()],
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },
  build: {
    target: ["chrome79"],
    // One vendor chunk: a TV loads the whole app once and keeps it; route
    // chunks would only add round trips on a slow panel.
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          if (id.includes("/hls.js/")) return "hls";
          if (id.includes("/node_modules/")) return "vendor";
        },
      },
    },
  },
  server: {
    host: true,
    port: 5174,
  },
});
