import { test, expect } from "./fixtures/auth";

test.describe("Admin libraries", () => {
  test("loads with heading and add button", async ({ page }) => {
    await page.goto("/admin/libraries", { waitUntil: "networkidle" });

    await expect(
      page.locator("h1", { hasText: "Libraries" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("button", { hasText: "Add Library" }),
    ).toBeVisible();
  });

  test("shows library table headers", async ({ page }) => {
    await page.goto("/admin/libraries", { waitUntil: "networkidle" });

    const headers = ["Name", "Type", "Paths", "Items", "Last Scanned", "Next Scan", "Actions"];
    for (const h of headers) {
      await expect(
        page.locator("th", { hasText: h }),
      ).toBeVisible({ timeout: 5_000 });
    }
  });

  test("add library button opens modal", async ({ page }) => {
    await page.goto("/admin/libraries", { waitUntil: "networkidle" });

    await page.click("button:has-text('Add Library')");

    await expect(
      page.locator("text=Add Library").last(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("trigger library scan shows toast", async ({ page }) => {
    await page.goto("/admin/libraries", { waitUntil: "networkidle" });

    await expect(
      page.locator('[data-testid="libraries-page"]'),
    ).toBeVisible({ timeout: 10_000 });

    const scanBtn = page.locator("button", { hasText: /^Scan$/ }).first();
    await expect(scanBtn).toBeVisible({ timeout: 10_000 });

    await scanBtn.click();

    await expect(
      page.locator("text=Scan started").or(page.locator("text=Scanning...")),
    ).toBeVisible({ timeout: 15_000 });
  });
});

test.describe("Library access restriction", () => {
  test.describe.configure({ mode: "serial" });

  const testUsername = `e2e_libaccess_${Date.now()}`;
  const testPassword = "TestPassword1234";
  let testUserId: string;

  test("setup: create non-admin test user", async ({ page }) => {
    // Extract token from the authed page session
    const token = await page.evaluate(() => {
      const raw = localStorage.getItem("auth-storage");
      if (!raw) return null;
      try {
        const parsed = JSON.parse(raw);
        return parsed?.state?.accessToken ?? null;
      } catch {
        return null;
      }
    });
    expect(token).toBeTruthy();

    const res = await page.request.post("/api/v1/users", {
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      data: { username: testUsername, password: testPassword, isAdmin: false },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    testUserId = body.id;
    expect(testUserId).toBeTruthy();
  });

  test("library access modal opens for non-admin user", async ({ page }) => {
    test.skip(!testUserId, "Test user not created");

    await page.goto("/admin/users", { waitUntil: "networkidle" });
    await expect(page.locator("h2", { hasText: "Users" })).toBeVisible({ timeout: 10_000 });

    const row = page.locator("tr", { hasText: testUsername });
    await expect(row).toBeVisible({ timeout: 10_000 });

    const libBtn = row.locator("button", { hasText: "Libraries" });
    await expect(libBtn).toBeVisible({ timeout: 5_000 });
    await libBtn.click();

    await expect(
      page.locator("text=Library Access"),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("restrict user to specific libraries and verify saved", async ({ page }) => {
    test.skip(!testUserId, "Test user not created");

    await page.goto("/admin/users", { waitUntil: "networkidle" });
    await expect(page.locator("h2", { hasText: "Users" })).toBeVisible({ timeout: 10_000 });

    const row = page.locator("tr", { hasText: testUsername });
    await expect(row).toBeVisible({ timeout: 10_000 });

    const libBtn = row.locator("button", { hasText: "Libraries" });
    await libBtn.click();

    await expect(
      page.locator("text=Library Access"),
    ).toBeVisible({ timeout: 10_000 });

    // Wait for the modal content to load (checkboxes)
    await page.locator('input[type="checkbox"]').first().waitFor({ timeout: 10_000 }).catch(() => {});

    // The modal has library checkboxes — look inside the modal dialog
    const checkboxes = page.locator('input[type="checkbox"]');
    const count = await checkboxes.count();
    test.skip(count === 0, "No libraries available to restrict");

    // Check the first library checkbox (if not already checked)
    const firstCheckbox = checkboxes.first();
    if (!(await firstCheckbox.isChecked())) {
      await firstCheckbox.check();
    }

    // Click Save
    await page.locator("button", { hasText: "Save" }).click();

    // Toast success
    await expect(page.locator("text=Library access updated")).toBeVisible({ timeout: 10_000 });

    // Re-open modal and verify restriction persisted
    await page.waitForResponse((res) => res.url().includes("/api/v1/library-access") && res.status() === 200).catch(() => {});
    await libBtn.click();
    await expect(page.locator("text=Library Access")).toBeVisible({ timeout: 10_000 });
    await page.locator('input[type="checkbox"]').first().waitFor({ timeout: 10_000 }).catch(() => {});

    await expect(
      page.locator("text=/can only access \\d+ selected/"),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("clear restrictions restores unrestricted access", async ({ page }) => {
    test.skip(!testUserId, "Test user not created");

    await page.goto("/admin/users", { waitUntil: "networkidle" });
    await expect(page.locator("h2", { hasText: "Users" })).toBeVisible({ timeout: 10_000 });

    const row = page.locator("tr", { hasText: testUsername });
    await expect(row).toBeVisible({ timeout: 10_000 });

    const libBtn = row.locator("button", { hasText: "Libraries" });
    await libBtn.click();

    await expect(page.locator("text=Library Access")).toBeVisible({ timeout: 10_000 });
    await page.locator('input[type="checkbox"]').first().waitFor({ timeout: 10_000 }).catch(() => {});

    // Clear all
    const clearBtn = page.locator("button", { hasText: "Clear all" });
    if (await clearBtn.isVisible()) {
      await clearBtn.click();
    }

    await page.locator("button", { hasText: "Save" }).click();
    await expect(page.locator("text=Library access updated")).toBeVisible({ timeout: 10_000 });
  });

  test("cleanup: delete test user", async ({ page }) => {
    if (!testUserId) return;

    const token = await page.evaluate(() => {
      const raw = localStorage.getItem("auth-storage");
      if (!raw) return null;
      try {
        return JSON.parse(raw)?.state?.accessToken ?? null;
      } catch {
        return null;
      }
    });
    if (!token) return;

    await page.request.delete(`/api/v1/users/${testUserId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  });
});
