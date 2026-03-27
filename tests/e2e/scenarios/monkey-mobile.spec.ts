import { test, expect } from "../fixtures/auth";
import { runMonkey } from "./shared/monkey";

/**
 * Mobile monkey: 100 runs with mobile viewport + touch
 */

test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

const RUNS = parseInt(process.env.MONKEY_MOBILE_RUNS ?? "100", 10);
const STEPS = parseInt(process.env.MONKEY_STEPS ?? "8", 10);

const ENTRIES = [
  "/home", "/home", "/library", "/favorites",
  "/collections", "/playlists", "/profile",
  "/admin", "/admin/libraries", "/admin/feeds",
];

for (let i = 0; i < RUNS; i++) {
  const entry = ENTRIES[i % ENTRIES.length];
  test(`mobile-monkey #${i + 1} — ${entry}`, async ({ page }) => {
    const result = await runMonkey(page, entry, STEPS);
    const log = result.steps.join("\n");
    expect(result.jsErrors, `JS:\n${result.jsErrors.join("\n")}\n\n${log}`).toHaveLength(0);
    expect(result.serverErrors, `5xx:\n${result.serverErrors.join("\n")}\n\n${log}`).toHaveLength(0);
    expect(result.blankPage, `Blank!\n\n${log}`).toBe(false);
  });
}
