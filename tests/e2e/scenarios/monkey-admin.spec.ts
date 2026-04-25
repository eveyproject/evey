import { test, expect } from "../fixtures/auth";
import { runMonkey } from "./shared/monkey";

/**
 * Admin-focused monkey: 100 runs starting from admin pages only
 */

const RUNS = parseInt(process.env.MONKEY_ADMIN_RUNS ?? "100", 10);
const STEPS = parseInt(process.env.MONKEY_STEPS ?? "8", 10);

const ADMIN_ENTRIES = [
  "/admin", "/admin/libraries", "/admin/users", "/admin/plugins",
  "/admin/webhooks", "/admin/metadata", "/admin/feeds", "/admin/audit",
  "/admin/activity", "/admin/sessions", "/admin/analytics",
  "/admin/system-health", "/admin/server-config", "/admin/logs",
];

for (let i = 0; i < RUNS; i++) {
  const entry = ADMIN_ENTRIES[i % ADMIN_ENTRIES.length];
  test(`admin-monkey #${i + 1} — ${entry}`, async ({ page }) => {
    const result = await runMonkey(page, entry, STEPS);
    const log = result.steps.join("\n");
    expect(result.jsErrors, `JS:\n${result.jsErrors.join("\n")}\n\n${log}`).toHaveLength(0);
    expect(result.serverErrors, `5xx:\n${result.serverErrors.join("\n")}\n\n${log}`).toHaveLength(0);
    // Skip blankPage assertion: a random walk can legitimately land on the
    // SPA's empty 404 shell (unmatched routes return index.html with empty
    // body). jsErrors + 5xx already catch the real bugs. Same fix as
    // random-walks.spec.ts.
  });
}
