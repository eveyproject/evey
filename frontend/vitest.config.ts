import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    projects: ["apps/web", "apps/mobile", "apps/tv", "packages/store"],
  },
});
