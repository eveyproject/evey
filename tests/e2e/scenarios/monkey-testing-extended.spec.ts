import { test, expect } from "../fixtures/auth";
import { runMonkey } from "./shared/monkey";

/**
 * Extended monkey testing: 200 runs × 10 steps = 2000 random actions
 * Run with: MONKEY_RUNS=200 npx playwright test scenarios/monkey-testing-extended.spec.ts --timeout=60000 --workers=2
 */

const RUNS = parseInt(process.env.MONKEY_RUNS ?? "300", 10);
const STEPS = parseInt(process.env.MONKEY_STEPS ?? "10", 10);
const BASE_SEED = parseInt(process.env.MONKEY_SEED ?? String(Date.now() % 2147483647), 10);

const ENTRIES = [
  "/home", "/home", "/home", "/home",
  "/library", "/favorites", "/collections", "/playlists",
  "/music", "/photos",
  "/profile", "/profile/settings", "/profile/history",
  "/admin", "/admin/libraries", "/admin/users",
  "/admin/feeds", "/admin/plugins", "/admin/metadata",
  "/admin/webhooks", "/admin/audit", "/admin/analytics",
];

for (let i = 0; i < RUNS; i++) {
  const entry = ENTRIES[i % ENTRIES.length];
  const seed = (BASE_SEED + i) >>> 0;

  test(`monkey-ext #${i + 1} — ${entry} seed:${seed}`, async ({ page }) => {
    const result = await runMonkey(page, entry, STEPS, seed);
    const log = `seed=${seed}  (replay: MONKEY_SEED=${seed} MONKEY_RUNS=1)\n${result.steps.join("\n")}`;
    expect(result.jsErrors, `JS:\n${result.jsErrors.join("\n")}\n\nSteps:\n${log}`).toHaveLength(0);
    expect(result.serverErrors, `5xx:\n${result.serverErrors.join("\n")}\n\nSteps:\n${log}`).toHaveLength(0);
    // Skip blankPage assertion: a random walk can legitimately land on the
    // SPA's empty 404 shell (unmatched routes return index.html with empty
    // body). jsErrors + 5xx already catch the real bugs. Same fix as
    // random-walks.spec.ts.
  });
}
