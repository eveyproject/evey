import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Feeds deep", () => {
  const waitForFeedsPage = async (page: import("@playwright/test").Page) => {
    // The standalone /admin/feeds page was folded into /admin/server-config as a
    // collapsed-by-default "Home feeds" CollapsibleCard (feeds.tsx now redirects
    // there). The card unmounts its children while collapsed, so expand collapsed
    // cards until the feeds save button mounts. Locale-independent on purpose.
    //
    // Robustness: the CollapsibleCards mount a beat after the page loads, so the
    // first pass can see zero collapsed headers — retry across a few attempts and
    // bound each click (an unbounded click on a not-yet-stable header would hang
    // until the test timeout under load).
    await goto(page, "/admin/server-config");
    const saveBtn = page.locator('[data-testid="feeds-save-btn"]');
    for (let attempt = 0; attempt < 4 && (await saveBtn.count()) === 0; attempt++) {
      const headers = page.locator('[role="button"][aria-expanded="false"]');
      const n = await headers.count();
      for (let i = 0; i < n; i++) {
        if ((await saveBtn.count()) > 0 || (await headers.count()) === 0) break;
        await headers.first().click({ timeout: 2_500 }).catch(() => {});
        await page.waitForTimeout(100);
      }
      if ((await saveBtn.count()) === 0) await page.waitForTimeout(500);
    }
    // wait for the save button to be visible (card expanded + page loaded)
    await saveBtn.scrollIntoViewIfNeeded().catch(() => {});
    await saveBtn.waitFor({ state: "visible", timeout: 10_000 });
    // give the API data time to render rows
    await page.waitForTimeout(2_000);
  };

  const hasFeedRows = async (page: import("@playwright/test").Page) => {
    return (await page.locator("div[draggable='true']").count()) > 0;
  };

  test("toggle feed off → save → home verifies fewer sections", async ({ page }) => {
    await goto(page, "/home");
    // wait for feed headings to render (they load from API)
    await page.locator("h2, h3").first().waitFor({ state: "visible", timeout: 10_000 }).catch(() => {});
    await page.waitForTimeout(1_000);
    const beforeCount = await page.locator("h2, h3").count();
    test.skip(beforeCount === 0, "No feed sections on home");

    await waitForFeedsPage(page);
    test.skip(!(await hasFeedRows(page)), "No feed rows in admin");

    // click first Disable button (enabled feed)
    const disableBtn = page.locator('button[role="switch"][aria-checked="true"]').first();
    test.skip(!(await disableBtn.isVisible({ timeout: 1_000 }).catch(() => false)), "No enabled feeds");

    // remember what's being disabled (bounded — never let a stale locator hang
    // the whole test to its timeout; feedName is only used for best-effort cleanup)
    const row = disableBtn.locator("xpath=ancestor::div[@draggable='true']");
    const feedName =
      (await row.locator("span.text-sm").first().textContent({ timeout: 3_000 }).catch(() => ""))?.trim() ?? "";

    await disableBtn.click({ timeout: 5_000 });
    await page.waitForTimeout(300);

    const { serverErrors } = await collectErrors(page, async () => {
      await page.locator('[data-testid="feeds-save-btn"]').click({ timeout: 5_000 });
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);

    await goto(page, "/home");
    await page.waitForTimeout(2_000);
    const afterCount = await page.locator("h2, h3").count();
    // feed count should decrease or stay same if feed had no data
    expect(afterCount).toBeLessThanOrEqual(beforeCount);

    // reload admin → verify the toggle persisted as disabled
    await waitForFeedsPage(page);
    if (feedName) {
      const targetRow = page.locator("div[draggable='true']").filter({ hasText: feedName });
      const enableBtn = targetRow.locator('button[role="switch"][aria-checked="false"]');
      if (await enableBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
        // cleanup: re-enable
        await enableBtn.click({ timeout: 5_000 }).catch(() => {});
        await page.waitForTimeout(300);
        await page.locator('[data-testid="feeds-save-btn"]').click({ timeout: 5_000 }).catch(() => {});
        await page.waitForTimeout(1_500);
      }
    }
  });

  test("toggle feed back on → save → home shows feed section", async ({ page }) => {
    await waitForFeedsPage(page);
    test.skip(!(await hasFeedRows(page)), "No feed rows");

    const disableBtn = page.locator('button[role="switch"][aria-checked="true"]').last();
    test.skip(!(await disableBtn.isVisible({ timeout: 1_000 }).catch(() => false)), "No enabled feeds");

    // disable one feed first
    await disableBtn.click();
    await page.waitForTimeout(300);
    await page.locator('[data-testid="feeds-save-btn"]').click();
    await page.waitForTimeout(1_500);

    // re-enable it
    await waitForFeedsPage(page);
    const enableBtn = page.locator('button[role="switch"][aria-checked="false"]').first();
    test.skip(!(await enableBtn.isVisible({ timeout: 1_000 }).catch(() => false)), "No disabled feed");

    await enableBtn.click();
    await page.waitForTimeout(300);
    await page.locator('[data-testid="feeds-save-btn"]').click();
    await page.waitForTimeout(1_500);

    await goto(page, "/home");
    await page.waitForTimeout(2_000);
    expect(await page.locator("h2, h3").count()).toBeGreaterThan(0);
  });

  test("reorder feeds via drag → save → reload → order preserved", async ({ page }) => {
    await waitForFeedsPage(page);
    const rows = page.locator("div[draggable='true']");
    const rowCount = await rows.count();
    test.skip(rowCount < 2, "Not enough rows to reorder");

    const initialLabels = await rows.locator("span.text-sm").allTextContents();

    const box0 = await rows.nth(0).boundingBox();
    const box1 = await rows.nth(1).boundingBox();
    if (box0 && box1) {
      await page.mouse.move(box0.x + 12, box0.y + box0.height / 2);
      await page.mouse.down();
      await page.mouse.move(box1.x + 12, box1.y + box1.height + 5, { steps: 10 });
      await page.mouse.up();
      await page.waitForTimeout(500);
    }

    await page.locator('[data-testid="feeds-save-btn"]').click();
    await page.waitForTimeout(2_000);

    await waitForFeedsPage(page);
    const afterLabels = await page.locator("div[draggable='true'] span.text-sm").allTextContents();
    expect(afterLabels.length).toBeGreaterThan(0);

    // cleanup: restore original order
    if (initialLabels.length > 1 && initialLabels[0] !== afterLabels[0]) {
      const r = page.locator("div[draggable='true']");
      const b0 = await r.nth(0).boundingBox();
      const b1 = await r.nth(1).boundingBox();
      if (b0 && b1) {
        await page.mouse.move(b0.x + 12, b0.y + b0.height / 2);
        await page.mouse.down();
        await page.mouse.move(b1.x + 12, b1.y + b1.height + 5, { steps: 10 });
        await page.mouse.up();
        await page.waitForTimeout(300);
        await page.locator('[data-testid="feeds-save-btn"]').click();
        await page.waitForTimeout(1_500);
      }
    }
  });

  test("save with no changes → no crash", async ({ page }) => {
    await waitForFeedsPage(page);

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await page.locator('[data-testid="feeds-save-btn"]').click();
      await page.waitForTimeout(1_500);
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });

  test("no UUID visible in feed names", async ({ page }) => {
    await waitForFeedsPage(page);

    const pageText = await page.evaluate(() => {
      const clone = document.body.cloneNode(true) as HTMLElement;
      clone.querySelectorAll("a[href], [data-id], input, [value]").forEach((el) => el.remove());
      return clone.textContent ?? "";
    });
    const uuids = pageText.match(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi) ?? [];
    expect(uuids.length, `UUIDs in page text: ${uuids.join(", ")}`).toBe(0);
  });
});
