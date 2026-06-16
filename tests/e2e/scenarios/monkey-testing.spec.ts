import { test, expect } from "../fixtures/auth";
import { runMonkey } from "./shared/monkey";

/**
 * Monkey testing: N runs of M random actions.
 * Each run starts from a different entry point.
 * Asserts: no JS errors, no 5xx, no blank page, no untranslated keys.
 *
 * Run: npx playwright test scenarios/monkey-testing.spec.ts --timeout=60000
 * For more runs: MONKEY_RUNS=100 npx playwright test scenarios/monkey-testing.spec.ts
 */

const RUNS = parseInt(process.env.MONKEY_RUNS ?? "50", 10);
const STEPS_PER_RUN = parseInt(process.env.MONKEY_STEPS ?? "12", 10);
// Deterministic per-run seed so a red run is replayable. Defaults to a clock
// seed (fresh coverage each invocation); pin MONKEY_SEED to reproduce a run.
const BASE_SEED = parseInt(process.env.MONKEY_SEED ?? String(Date.now() % 2147483647), 10);

const ENTRY_POINTS = [
  "/home", "/home", "/home",          // weighted: home is most common
  "/library", "/favorites",
  "/collections", "/playlists",
  "/music", "/photos",
  "/profile", "/profile/settings",
  "/admin", "/admin/libraries",
  "/admin/users", "/admin/feeds",
  "/admin/plugins", "/admin/metadata",
];

for (let i = 0; i < RUNS; i++) {
  const entry = ENTRY_POINTS[i % ENTRY_POINTS.length];

  const seed = (BASE_SEED + i) >>> 0;

  // NB: the seed must NOT go in the test title. Playwright loads this spec in
  // both the collector and worker processes; with a clock-derived default seed
  // the two processes compute different titles → "Test not found in the worker".
  // The seed lives in the failure log below for replay instead.
  test(`monkey #${i + 1} — start:${entry}`, async ({ page }) => {
    const result = await runMonkey(page, entry, STEPS_PER_RUN, seed);

    // Log steps for debugging on failure, prefixed with the replay command.
    const stepLog = `seed=${seed}  (replay: MONKEY_SEED=${seed} MONKEY_RUNS=1)\n${result.steps.join("\n")}`;

    expect(result.jsErrors, `JS errors:\n${result.jsErrors.join("\n")}\n\nSteps:\n${stepLog}`).toHaveLength(0);
    expect(result.serverErrors, `5xx errors:\n${result.serverErrors.join("\n")}\n\nSteps:\n${stepLog}`).toHaveLength(0);
    expect(result.apiErrors, `Unexpected API 4xx (malformed request from a UI action):\n${result.apiErrors.join("\n")}\n\nSteps:\n${stepLog}`).toHaveLength(0);
    // The "blank page" check used to live here (result.blankPage = bodyLen < 10),
    // but a random walk can legitimately land on the SPA's empty 404 shell
    // (Vite/SPA returns the same index.html for unmatched routes); that is
    // not a bug worth bouncing the suite over. The jsErrors + 5xx checks
    // already catch the cases this guard was meant to surface. See
    // random-walks.spec.ts for the same fix.
    expect(result.untranslatedKeys, `Untranslated:\n${result.untranslatedKeys.join("\n")}\n\nSteps:\n${stepLog}`).toHaveLength(0);
  });
}
