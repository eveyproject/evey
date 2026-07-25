import { defineConfig } from "vite";
import { resolve } from "path";

// Build STANDALONE du Web Receiver Cast, pour l'hébergement statique hors
// serveur (GitHub Pages de l'org - servi à la racine, les chemins /wasm/ et
// /assets/ absolus tiennent). Une seule entrée ; publicDir coupé : le public/
// de l'app pèse lourd et seuls les artefacts wasm sont nécessaires - copiés
// par l'assemblage (`pnpm run build:cast-receiver` puis cp de public/wasm).
export default defineConfig({
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },
  publicDir: false,
  build: {
    outDir: "dist-cast-receiver",
    emptyOutDir: true,
    rollupOptions: {
      input: {
        castReceiver: resolve(__dirname, "cast-receiver.html"),
      },
    },
  },
});
