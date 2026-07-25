import { defineConfig } from "vite";
import { resolve } from "path";

// STANDALONE build of the Cast Web Receiver, for static hosting off the
// server (org's GitHub Pages - served at the root, so absolute /wasm/ and
// /assets/ paths hold). Single entry; publicDir disabled: the app's public/
// is heavy and only the wasm artifacts are needed - copied by the assembly
// step (`pnpm run build:cast-receiver` then cp from public/wasm).
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
