import { defineConfig } from "vitest/config";

// fetcher.test.ts covers the token-refresh single-flight, and was unrunnable
// until now: the package had no vitest config and was absent from the root
// `projects` list, so no command could reach it. A DOM because the fetcher
// touches document.cookie.
export default defineConfig({
  test: {
    globals: true,
    environment: "happy-dom",
    css: false,
  },
});
