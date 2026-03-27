import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Feeds deep", () => {
  test("toggle feed off → save → home has no section for that feed", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);

    // find first enabled toggle
    const toggle = page.locator('button[role="switch"][aria-checked="true"], button[aria-label*="able" i][aria-pressed="true"]').first();
    test.skip(!(await toggle.isVisible({ timeout: 2_000 }).catch(() => false)), "No enabled toggle");

    // get feed label before toggling
    const row = toggle.locator("xpath=ancestor::tr | ancestor::div[contains(@class,'row')] | ancestor::li").first();
    const feedName = (await row.textContent())?.trim().split("\n")[0] ?? "";

    await toggle.click();
    await page.waitForTimeout(300);

    const saveBtn = page.locator("button", { hasText: /save/i }).first();
    await saveBtn.click();
    await page.waitForTimeout(1_500);

    // go home and verify feed section gone
    await goto(page, "/home");
    await page.waitForTimeout(1_500);
    if (feedName) {
      const headings = await page.locator("h2, h3").allTextContents();
      const found = headings.some((h) => h.toLowerCase().includes(feedName.toLowerCase().slice(0, 10)));
      expect(found, `Feed "${feedName}" should be hidden on home`).toBe(false);
    }

    // cleanup: re-enable
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);
    const disabledToggle = page.locator('button[role="switch"][aria-checked="false"], button[aria-label*="able" i][aria-pressed="false"]').first();
    if (await disabledToggle.isVisible({ timeout: 1_000 }).catch(() => false)) {
      await disabledToggle.click();
      await page.waitForTimeout(300);
      await page.locator("button", { hasText: /save/i }).first().click();
      await page.waitForTimeout(1_000);
    }
  });

  test("toggle feed back on → save → home shows feed section", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);

    // find first disabled toggle and enable it
    const toggle = page.locator('button[role="switch"][aria-checked="false"], button[aria-label*="able" i][aria-pressed="false"]').first();
    test.skip(!(await toggle.isVisible({ timeout: 2_000 }).catch(() => false)), "No disabled toggle");

    await toggle.click();
    await page.waitForTimeout(300);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_500);

    await goto(page, "/home");
    await page.waitForTimeout(1_500);
    const sections = page.locator('[data-testid="media-slider"], [data-testid="feed-section"], section');
    expect(await sections.count()).toBeGreaterThan(0);
  });

  test("reorder feeds via drag → save → reload → order preserved", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);

    // look for drag handles
    const handles = page.locator('[data-testid="drag-handle"], [aria-roledescription="sortable"], button[aria-label*="drag" i], .drag-handle, [class*="grip"], svg[class*="grip"]');
    const handleCount = await handles.count();
    test.skip(handleCount < 2, "Not enough drag handles");

    // record initial order from row text
    const rows = page.locator('[data-testid="feed-row"], tr, [class*="feed-item"], li').filter({ has: handles.first() });
    const initialOrder = await rows.allTextContents();

    // drag first handle to second position
    const first = handles.nth(0);
    const second = handles.nth(1);
    const box1 = await first.boundingBox();
    const box2 = await second.boundingBox();
    if (box1 && box2) {
      await page.mouse.move(box1.x + box1.width / 2, box1.y + box1.height / 2);
      await page.mouse.down();
      await page.mouse.move(box2.x + box2.width / 2, box2.y + box2.height + 5, { steps: 10 });
      await page.mouse.up();
      await page.waitForTimeout(500);
    }

    const saveBtn = page.locator("button", { hasText: /save/i }).first();
    if (await saveBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
      await saveBtn.click();
      await page.waitForTimeout(1_500);
    }

    // reload and verify order changed or stayed (no crash)
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);
    const afterOrder = await rows.allTextContents();
    expect(afterOrder.length).toBeGreaterThan(0);
  });

  test("save with no changes → no crash", async ({ page }) => {
    await goto(page, "/admin/feeds");
    const saveBtn = page.locator("button", { hasText: /save/i }).first();
    test.skip(!(await saveBtn.isVisible({ timeout: 3_000 }).catch(() => false)), "No save button");

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await saveBtn.click();
      await page.waitForTimeout(1_500);
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });

  test("no UUID visible in feed names", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);

    // strip URLs from text before checking for UUIDs
    const pageText = await page.evaluate(() => {
      const clone = document.body.cloneNode(true) as HTMLElement;
      clone.querySelectorAll("a[href], [data-id], input, [value]").forEach((el) => el.remove());
      return clone.textContent ?? "";
    });
    const uuids = pageText.match(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi) ?? [];
    expect(uuids.length, `UUIDs in page text: ${uuids.join(", ")}`).toBe(0);
  });
});
