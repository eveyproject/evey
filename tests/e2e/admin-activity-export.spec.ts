import { test, expect } from "./fixtures/auth";

// The admin activity export used to ask for a single page of 10000 rows. No
// endpoint has ever served that: GetPlayHistory clamps to maxPageSize and the
// audit log clamps in its repository, both at 500. So "Export JSON" wrote a
// file holding the first 500 rows, under the name of the whole log, with
// nothing on screen or in the file to say otherwise. Measured on the dev
// database: 500 rows exported out of 6109 - 92% of the audit log missing, and
// no way for the operator to notice.
//
// A unit test can prove the paging helper terminates. Only this can prove the
// button actually hands over a complete file.
test.describe("Admin activity export", () => {
  test("the audit log export contains every row, not just the first page", async ({ page }) => {
    await page.goto("/admin/activity", { waitUntil: "domcontentloaded" });

    // Tab labels follow the profile's language; match both catalogues.
    const auditTab = page.getByRole("button", { name: /audit log|journal d'audit/i });
    await auditTab.waitFor({ state: "visible", timeout: 15_000 });
    await auditTab.click();

    const exportButton = page.getByRole("button", { name: /export json|exporter json/i });
    await exportButton.waitFor({ state: "visible", timeout: 15_000 });

    const [download] = await Promise.all([
      page.waitForEvent("download", { timeout: 60_000 }),
      exportButton.click(),
    ]);

    expect(download.suggestedFilename()).toMatch(/^audit-log-\d{4}-\d{2}-\d{2}\.json$/);

    const path = await download.path();
    expect(path).toBeTruthy();
    const fs = await import("node:fs/promises");
    const payload = JSON.parse(await fs.readFile(path!, "utf8")) as {
      entries?: unknown[];
      total?: number;
    };

    const entries = payload.entries ?? [];
    expect(entries.length, "the export must hold rows").toBeGreaterThan(0);

    // The claim under test: the file holds what the server says exists, rather
    // than stopping at whatever one request happened to return.
    expect(payload.total, "the export must carry the server's own total").toBeGreaterThan(0);
    expect(
      entries.length,
      `exported ${entries.length} of ${payload.total} rows - a truncated export is indistinguishable from a complete one once downloaded`,
    ).toBe(payload.total);

    // And it must have taken more than one request to get there, otherwise this
    // passes trivially on a database small enough to fit in a single page.
    expect(payload.total, "dev database too small to exercise paging").toBeGreaterThan(500);
  });
});
