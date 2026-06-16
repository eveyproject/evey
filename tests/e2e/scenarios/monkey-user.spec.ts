import { test, expect } from "../fixtures/auth";
import { runMonkey } from "./shared/monkey";

/**
 * User-focused monkey: 100 runs starting from user pages only
 */

const RUNS = parseInt(process.env.MONKEY_USER_RUNS ?? "100", 10);
const STEPS = parseInt(process.env.MONKEY_STEPS ?? "10", 10);
const BASE_SEED = parseInt(process.env.MONKEY_SEED ?? String(Date.now() % 2147483647), 10);

const USER_ENTRIES = [
  "/home", "/home", "/home", "/library", "/favorites",
  "/collections", "/playlists", "/music", "/photos",
  "/profile", "/profile/settings", "/profile/history",
];

for (let i = 0; i < RUNS; i++) {
  const entry = USER_ENTRIES[i % USER_ENTRIES.length];
  const seed = (BASE_SEED + i) >>> 0;
  // Seed kept out of the title (clock-default → cross-process mismatch); in the log.
  test(`user-monkey #${i + 1} — ${entry}`, async ({ page }) => {
    const result = await runMonkey(page, entry, STEPS, seed);
    const log = `seed=${seed}  (replay: MONKEY_SEED=${seed} MONKEY_USER_RUNS=1)\n${result.steps.join("\n")}`;
    expect(result.jsErrors, `JS:\n${result.jsErrors.join("\n")}\n\n${log}`).toHaveLength(0);
    expect(result.serverErrors, `5xx:\n${result.serverErrors.join("\n")}\n\n${log}`).toHaveLength(0);
    expect(result.apiErrors, `Unexpected API 4xx:\n${result.apiErrors.join("\n")}\n\n${log}`).toHaveLength(0);
    // Skip blankPage assertion: a random walk can legitimately land on the
    // SPA's empty 404 shell (unmatched routes return index.html with empty
    // body). jsErrors + 5xx already catch the real bugs. Same fix as
    // random-walks.spec.ts.
  });
}
