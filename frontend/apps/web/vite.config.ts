import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { TanStackRouterVite } from "@tanstack/router-vite-plugin";
import { resolve } from "path";

export default defineConfig({
  plugins: [TanStackRouterVite(), react(), tailwindcss()],
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ["react", "react-dom", "@tanstack/react-query", "@tanstack/react-router"],
          hls: ["hls.js"],
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
      port: process.env.VITE_HMR_PORT ? Number(process.env.VITE_HMR_PORT) : undefined,
      protocol: process.env.VITE_HMR_PROTOCOL as "ws" | "wss" | undefined,
    } : true,
    proxy: {
      "/api": {
        target: process.env.VITE_API_TARGET ?? "http://localhost:9091",
        changeOrigin: true,
      },
    },
  },
});
