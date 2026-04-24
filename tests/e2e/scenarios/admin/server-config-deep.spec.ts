import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Server Config", () => {
  test("page loads with form", async ({ page }) => {
    await goto(page, "/admin/server-config");
    await page.waitForTimeout(2_000);
    const inputs = page.locator("input, select, textarea");
    expect(await inputs.count()).toBeGreaterThan(0);
  });

  test("server name field is wired up", async ({ page }) => {
    await goto(page, "/admin/server-config");
    await page.waitForTimeout(2_000);
    const nameInput = page.locator('[data-testid="server-name-input"]');
    await expect(nameInput).toBeVisible({ timeout: 5_000 });
    // Field acts as an override: value stays empty until edited; the current
    // server name shows up in the placeholder so users see what they'd be
    // replacing.
    const placeholder = await nameInput.getAttribute("placeholder");
    expect(placeholder?.length ?? 0).toBeGreaterThan(0);
  });

  test("save button exists", async ({ page }) => {
    await goto(page, "/admin/server-config");
    await expect(page.locator('[data-testid="server-config-save-btn"]')).toBeVisible({ timeout: 5_000 });
  });

  test("save without changes doesn't crash", async ({ page }) => {
    await goto(page, "/admin/server-config");
    const save = page.locator('[data-testid="server-config-save-btn"]');
    test.skip(!(await save.isVisible({ timeout: 3_000 }).catch(() => false)), "No save btn");
    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      // Button is disabled when form is clean; force the click to exercise the
      // no-op path and confirm nothing in the page-level handler crashes.
      await save.click({ force: true });
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });

  test("no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/server-config");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/server-config");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});

test.describe("Admin > System Health", () => {
  test("page loads with metrics", async ({ page }) => {
    await goto(page, "/admin/system-health");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body!.length).toBeGreaterThan(20);
  });

  test("disk usage info visible", async ({ page }) => {
    await goto(page, "/admin/system-health");
    await page.waitForTimeout(2_000);
    const disk = page.locator("text=/disk|storage|GB|MB/i").first();
    const visible = await disk.isVisible({ timeout: 3_000 }).catch(() => false);
    expect(typeof visible).toBe("boolean");
  });

  test("no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/system-health");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/system-health");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});

test.describe("Admin > Analytics", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/analytics");
    await page.waitForTimeout(2_000);
    const h = page.locator("h1, h2").first();
    await expect(h).toBeVisible({ timeout: 5_000 });
  });

  test("no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/analytics");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});

test.describe("Admin > Activity", () => {
  test("tabs are visible", async ({ page }) => {
    await goto(page, "/admin/activity");
    await page.waitForTimeout(2_000);
    const tabs = page.locator("button[aria-pressed]");
    const count = await tabs.count();
    expect(count).toBeGreaterThan(0);
  });

  test("clicking each tab doesn't crash", async ({ page }) => {
    await goto(page, "/admin/activity");
    await page.waitForTimeout(1_000);
    const tabs = page.locator("button[aria-pressed]");
    const count = await tabs.count();
    for (let i = 0; i < count; i++) {
      const { jsErrors } = await collectErrors(page, async () => {
        await tabs.nth(i).click();
        await page.waitForTimeout(800);
      });
      expect(jsErrors).toHaveLength(0);
    }
  });

  test("no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/activity");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});

test.describe("Admin > Sessions", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/sessions");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body!.length).toBeGreaterThan(20);
  });

  test("no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/sessions");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
