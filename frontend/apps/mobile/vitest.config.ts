import { defineConfig } from "vitest/config";
import { resolve } from "path";

export default defineConfig({
  resolve: {
    alias: {
      "@evey/api": resolve(__dirname, "../../packages/api/src/fetcher.ts"),
      "react-native": resolve(__dirname, "./src/test/__mocks__/react-native.ts"),
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    css: false,
  },
});
