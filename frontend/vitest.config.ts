import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    projects: ["apps/web", "apps/mobile", "apps/tv", "packages/store", "packages/client", "packages/api", "packages/theme", "packages/media-caps", "packages/voice-search", "packages/offline-audio"],
  },
});
