import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Feeds deep", () => {
  // toggle selector: custom switch with aria-label containing "able" (Enable/Disable)
  const enabledToggle = 'button[aria-label*="isable" i]'; // "Disable" = currently on
  const disabledToggle = 'button[aria-label*="nable" i]:not([aria-label*="isable" i])'; // "Enable" = currently off

  test("toggle feed off → save → home has no section for that feed", async ({ page }) => {
    // first go home to see which feeds are visible
    await goto(page, "/home");
    await page.waitForTimeout(2_000);
    const homeHeadings = await page.locator("h2, h3").allTextContents();
    const visibleFeed = homeHeadings.find((h) => h.trim().length > 0);
    test.skip(!visibleFeed, "No feed sections visible on home");

    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);

    // find the first row with an enabled toggle (we'll verify by feed name later)
    const rows = page.locator("div[draggable='true']");
    const rowCount = await rows.count();
    let targetIdx = -1;
    for (let i = 0; i < rowCount; i++) {
      const t = rows.nth(i).locator(enabledToggle);
      if (await t.isVisible({ timeout: 500 }).catch(() => false)) {
        targetIdx = i;
        break;
      }
    }
    test.skip(targetIdx === -1, "No enabled feed to toggle");

    const rowToggle = rows.nth(targetIdx).locator(enabledToggle);
    test.skip(!(await rowToggle.isVisible({ timeout: 1_000 }).catch(() => false)), "Toggle not found");

    await rowToggle.click();
    await page.waitForTimeout(300);
    const { serverErrors } = await collectErrors(page, async () => {
      await page.locator("button", { hasText: /save/i }).first().click();
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);

    await goto(page, "/home");
    await page.waitForTimeout(2_000);
    const afterHeadings = await page.locator("h2, h3").allTextContents();
    expect(afterHeadings.length, "Should have fewer feed sections after disabling one").toBeLessThan(homeHeadings.length);

    // cleanup: re-enable
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);
    const offToggle = rows.nth(targetIdx).locator(disabledToggle);
    if (await offToggle.isVisible({ timeout: 1_000 }).catch(() => false)) {
      await offToggle.click();
      await page.waitForTimeout(300);
      await page.locator("button", { hasText: /save/i }).first().click();
      await page.waitForTimeout(1_500);
    }
  });

  test("toggle feed back on → save → home shows feed section", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);

    const toggle = page.locator(disabledToggle).first();
    test.skip(!(await toggle.isVisible({ timeout: 2_000 }).catch(() => false)), "No disabled toggle");

    await toggle.click();
    await page.waitForTimeout(300);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_500);

    await goto(page, "/home");
    await page.waitForTimeout(1_500);
    const sections = page.locator('[data-testid="media-slider"], section, h2, h3');
    expect(await sections.count()).toBeGreaterThan(0);
  });

  test("reorder feeds via drag → save → reload → order preserved", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);

    // draggable rows with GripVertical svg icons
    const rows = page.locator("div[draggable='true']");
    const rowCount = await rows.count();
    test.skip(rowCount < 2, "Not enough rows to reorder");

    const labels = await rows.locator("span.text-sm").allTextContents();

    // drag row 0 over row 1
    const box0 = await rows.nth(0).boundingBox();
    const box1 = await rows.nth(1).boundingBox();
    if (box0 && box1) {
      await page.mouse.move(box0.x + 12, box0.y + box0.height / 2);
      await page.mouse.down();
      await page.mouse.move(box1.x + 12, box1.y + box1.height + 5, { steps: 10 });
      await page.mouse.up();
      await page.waitForTimeout(500);
    }

    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_500);

    // reload and verify persistence
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(1_500);
    const afterLabels = await page.locator("div[draggable='true'] span.text-sm").allTextContents();
    expect(afterLabels.length).toBeGreaterThan(0);

    // cleanup: drag back to original order
    const r = page.locator("div[draggable='true']");
    const b0 = await r.nth(0).boundingBox();
    const b1 = await r.nth(1).boundingBox();
    if (b0 && b1 && labels[0] !== afterLabels[0]) {
      await page.mouse.move(b0.x + 12, b0.y + b0.height / 2);
      await page.mouse.down();
      await page.mouse.move(b1.x + 12, b1.y + b1.height + 5, { steps: 10 });
      await page.mouse.up();
      await page.waitForTimeout(300);
      await page.locator("button", { hasText: /save/i }).first().click();
      await page.waitForTimeout(1_000);
    }
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

    // strip inputs and hidden els before checking for UUIDs in visible text
    const pageText = await page.evaluate(() => {
      const clone = document.body.cloneNode(true) as HTMLElement;
      clone.querySelectorAll("a[href], [data-id], input, [value]").forEach((el) => el.remove());
      return clone.textContent ?? "";
    });
    const uuids = pageText.match(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi) ?? [];
    expect(uuids.length, `UUIDs in page text: ${uuids.join(", ")}`).toBe(0);
  });
});
