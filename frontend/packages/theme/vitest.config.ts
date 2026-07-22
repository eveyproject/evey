import { defineConfig } from "vitest/config";

// The theme package is pure data plus a CSS generator; contrast.test.ts only
// does arithmetic on hex strings, so no DOM is needed.
export default defineConfig({
  test: {
    globals: true,
    environment: "node",
  },
});
