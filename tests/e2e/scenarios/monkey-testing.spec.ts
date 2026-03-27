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

  test(`monkey #${i + 1} — start:${entry}`, async ({ page }) => {
    const result = await runMonkey(page, entry, STEPS_PER_RUN);

    // Log steps for debugging on failure
    const stepLog = result.steps.join("\n");

    expect(result.jsErrors, `JS errors:\n${result.jsErrors.join("\n")}\n\nSteps:\n${stepLog}`).toHaveLength(0);
    expect(result.serverErrors, `5xx errors:\n${result.serverErrors.join("\n")}\n\nSteps:\n${stepLog}`).toHaveLength(0);
    expect(result.blankPage, `Blank page!\n\nSteps:\n${stepLog}`).toBe(false);
    expect(result.untranslatedKeys, `Untranslated:\n${result.untranslatedKeys.join("\n")}\n\nSteps:\n${stepLog}`).toHaveLength(0);
  });
}
