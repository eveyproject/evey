import { defineConfig } from "@playwright/test";
import { STORAGE_STATE } from "./global-setup";

// E2E login-rate-limit rule. The prod default is a tight EVEY_RATELIMIT_LOGIN_MAX=10
// (10 logins/min/IP) which, across a full suite of logins + token refreshes, trips
// LoginRL and snowballs into a 429→500 avalanche. The dev/test SERVER already raises
// it (docker-compose.override.yml → EVEY_RATELIMIT_LOGIN_MAX=1000) so the suite never
// throttles. Mirror that value into the RUNNER's env here (it runs on the host, which
// otherwise can't see the container's override) so specs reasoning about the limit -
// e.g. rate-limiting.spec - agree with the server and skip cleanly instead of firing
// a doomed burst. Keep this in sync with the compose override; CI sets both explicitly
// (and can drop it to the prod default to actually exercise the limiter).
process.env.EVEY_RATELIMIT_LOGIN_MAX ??= "1000";

export default defineConfig({
  globalSetup: "./global-setup.ts",
  globalTeardown: "./global-teardown.ts",
  testDir: ".",
  testMatch: "**/*.spec.ts",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  retries: 0,
  reporter: [["html", { open: "never" }]],
  use: {
    baseURL: process.env.BASE_URL ?? "https://evey.test",
    ignoreHTTPSErrors: true,
    // Reuse the single login from global-setup so tests don't each hammer
    // /auth/login (LoginRL 10/min/IP → 429→500 avalanche that produced mass
    // false failures on full-suite runs). Specs needing a logged-out context
    // override with test.use({ storageState: { cookies: [], origins: [] } }).
    storageState: STORAGE_STATE,
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
    {
      name: "firefox",
      use: { browserName: "firefox" },
    },
    // Safari's engine. Needed because the HLS master now advertises an
    // EXT-X-MEDIA audio group by default, and Safari is the strictest consumer
    // of it. Runs by default like the other two, which is a third of the suite's
    // wall clock - drop it with `--project=chromium --project=firefox` when that
    // matters. The runner needs its system deps once:
    //     npx playwright install --with-deps webkit
    // NB: WebKit on Linux is not Safari - no Apple native-HLS stack, GStreamer
    // decoding. It guards the manifest + MSE path, it does not replace a real
    // iOS/macOS run.
    {
      name: "webkit",
      use: { browserName: "webkit" },
    },
  ],
});
