import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { TanStackRouterVite } from "@tanstack/router-vite-plugin";
import { resolve } from "path";

export default defineConfig({
  plugins: [
    TanStackRouterVite({
      routeFileIgnorePattern: "\\.test\\.|^[A-Z][A-Za-z0-9]*\\.tsx$",
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },
  build: {
    rollupOptions: {
      // Deux pages : l'app SPA et le Web Receiver Cast custom (chargé par le
      // Chromecast à /cast-receiver.html, déclaré dans la Google Cast Console).
      input: {
        main: resolve(__dirname, "index.html"),
        castReceiver: resolve(__dirname, "cast-receiver.html"),
      },
      output: {
        // Vite 8 swapped rollup for rolldown, which exposes manualChunks
        // only as a function (the object form is silently dropped). Function
        // form works in both bundlers, so this stays compatible if Vite
        // ever rolls back.
        manualChunks(id: string) {
          if (id.includes("/hls.js/")) return "hls";
          if (id.includes("/react-gifted-charts/") || id.includes("/gifted-charts-core/")) return "charts";
          if (
            id.includes("/i18next/") ||
            id.includes("/react-i18next/") ||
            id.includes("/i18next-http-backend/") ||
            id.includes("/i18next-browser-languagedetector/")
          ) {
            return "i18n";
          }
          if (
            id.includes("/node_modules/react/") ||
            id.includes("/node_modules/react-dom/") ||
            id.includes("/@tanstack/react-query/") ||
            id.includes("/@tanstack/react-router/")
          ) {
            return "vendor";
          }
        },
      },
    },
  },
  server: {
    host: true,
    allowedHosts: process.env.VITE_ALLOWED_HOSTS ? process.env.VITE_ALLOWED_HOSTS.split(",") : true,
    watch: { usePolling: true, interval: 500 },
    hmr: process.env.VITE_HMR_HOST ? {
      host: process.env.VITE_HMR_HOST,
      clientPort: process.env.VITE_HMR_PORT ? Number(process.env.VITE_HMR_PORT) : undefined,
      protocol: process.env.VITE_HMR_PROTOCOL as "ws" | "wss" | undefined,
    } : true,
    proxy: {
      "/api": {
        target: process.env.VITE_API_TARGET ?? "http://localhost:9091",
        changeOrigin: true,
        // The SyncPlay room and the Play On device sockets live under /api:
        // without it the dev server never forwarded the upgrade.
        ws: true,
      },
    },
  },
});
