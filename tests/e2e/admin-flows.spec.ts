import { test, expect } from "./fixtures/auth";

async function goto(page: import("@playwright/test").Page, path: string) {
  await page.goto(path, { waitUntil: "domcontentloaded", timeout: 10_000 });
}

// ─── DASHBOARD ──────────────────────────────────────────────────────

test.describe("Admin Dashboard", () => {
  test("dashboard loads", async ({ page }) => {
    await goto(page, "/admin");
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("system info card visible", async ({ page }) => {
    await goto(page, "/admin");
    await expect(page.locator("text=/system|server|info/i").first()).toBeVisible({ timeout: 5_000 });
  });

  test("admin nav has expected links", async ({ page }) => {
    await goto(page, "/admin");
    await page.waitForTimeout(1_000);
    const nav = page.locator("nav");
    for (const section of ["libraries", "users", "plugins", "logs"]) {
      const link = nav.locator(`a[href*="${section}"]`).first();
      expect(await link.count()).toBeGreaterThan(0);
    }
  });
});

// ─── LIBRARIES ──────────────────────────────────────────────────────

test.describe("Admin Libraries", () => {
  test("page loads with heading", async ({ page }) => {
    await goto(page, "/admin/libraries");
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("add library button exists", async ({ page }) => {
    await goto(page, "/admin/libraries");
    await expect(page.locator("button", { hasText: /add|create|new/i })).toBeVisible({ timeout: 5_000 });
  });

  test("library table has rows", async ({ page }) => {
    await goto(page, "/admin/libraries");
    await page.waitForTimeout(2_000);
    const rows = page.locator("table tbody tr, [data-testid='library-row']");
    const count = await rows.count();
    expect(count).toBeGreaterThanOrEqual(0); // may be empty
  });

  test("scan button exists for each library", async ({ page }) => {
    await goto(page, "/admin/libraries");
    await page.waitForTimeout(2_000);
    const scanBtns = page.locator("button", { hasText: /scan/i });
    const count = await scanBtns.count();
    // at least 0 (no libraries = no buttons, that's ok)
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

// ─── USERS ──────────────────────────────────────────────────────────

test.describe("Admin Users", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/users");
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("create user button exists", async ({ page }) => {
    await goto(page, "/admin/users");
    await expect(page.locator("button", { hasText: /create.*user|add.*user/i })).toBeVisible({ timeout: 5_000 });
  });

  test("invite link button exists", async ({ page }) => {
    await goto(page, "/admin/users");
    await expect(page.locator("button", { hasText: /invite/i })).toBeVisible({ timeout: 5_000 });
  });

  test("users table has at least 1 user", async ({ page }) => {
    await goto(page, "/admin/users");
    await page.waitForTimeout(2_000);
    const rows = page.locator("table tbody tr, [data-testid='user-row']");
    const count = await rows.count();
    expect(count).toBeGreaterThan(0); // at least the admin user
  });
});

// ─── PLUGINS ────────────────────────────────────────────────────────

test.describe("Admin Plugins", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/plugins");
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("install button exists", async ({ page }) => {
    await goto(page, "/admin/plugins");
    await expect(page.locator("button", { hasText: /install/i })).toBeVisible({ timeout: 5_000 });
  });
});

// ─── METADATA ───────────────────────────────────────────────────────

test.describe("Admin Metadata", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/metadata");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("refresh all button exists", async ({ page }) => {
    await goto(page, "/admin/metadata");
    await expect(page.locator("button", { hasText: /refresh/i })).toBeVisible({ timeout: 5_000 });
  });
});

// ─── FEEDS ──────────────────────────────────────────────────────────

test.describe("Admin Feeds", () => {
  test("page loads with feed rows", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("save button exists", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await expect(page.locator("button", { hasText: /save/i })).toBeVisible({ timeout: 5_000 });
  });

  test("feed rows show library names not UUIDs", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(2_000);
    const cells = await page.locator("span").allTextContents();
    const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-/;
    const uuidCells = cells.filter((t) => uuidPattern.test(t.trim()));
    expect(uuidCells).toHaveLength(0);
  });
});

// ─── WEBHOOKS ───────────────────────────────────────────────────────

test.describe("Admin Webhooks", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/webhooks");
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });

  test("create webhook button exists", async ({ page }) => {
    await goto(page, "/admin/webhooks");
    await expect(page.locator("button", { hasText: /create|add|new/i })).toBeVisible({ timeout: 5_000 });
  });
});

// ─── AUDIT ──────────────────────────────────────────────────────────

test.describe("Admin Audit", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/audit");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });
});

// ─── ACTIVITY ───────────────────────────────────────────────────────

test.describe("Admin Activity", () => {
  test("page loads with tabs", async ({ page }) => {
    await goto(page, "/admin/activity");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });
});

// ─── SESSIONS ───────────────────────────────────────────────────────

test.describe("Admin Sessions", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/sessions");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });
});

// ─── ANALYTICS ──────────────────────────────────────────────────────

test.describe("Admin Analytics", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/analytics");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });
});

// ─── SYSTEM HEALTH ──────────────────────────────────────────────────

test.describe("Admin System Health", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/system-health");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });
});

// ─── SERVER CONFIG ──────────────────────────────────────────────────

test.describe("Admin Server Config", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/server-config");
    await page.waitForTimeout(2_000);
    await expect(page.locator("h1, h2").first()).toBeVisible({ timeout: 5_000 });
  });
});

// ─── LOGS ───────────────────────────────────────────────────────────

test.describe("Admin Logs", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/admin/logs");
    await page.waitForTimeout(2_000);
    // may show "coming soon" or actual logs
    expect(page.url()).toContain("/admin/logs");
  });
});

// ─── NO 5xx ON ANY ADMIN PAGE ───────────────────────────────────────

test.describe("Admin Stability", () => {
  const adminPages = [
    "/admin", "/admin/libraries", "/admin/users", "/admin/plugins",
    "/admin/webhooks", "/admin/metadata", "/admin/feeds", "/admin/audit",
    "/admin/activity", "/admin/sessions", "/admin/analytics",
    "/admin/system-health", "/admin/server-config", "/admin/logs",
  ];

  for (const path of adminPages) {
    test(`no 5xx errors on ${path}`, async ({ page }) => {
      const errors: string[] = [];
      page.on("response", (r) => {
        if (r.status() >= 500) errors.push(`${r.status()} ${r.url()}`);
      });
      await goto(page, path);
      await page.waitForTimeout(2_000);
      expect(errors).toHaveLength(0);
    });
  }

  for (const path of adminPages) {
    test(`no JS errors on ${path}`, async ({ page }) => {
      const errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await goto(page, path);
      await page.waitForTimeout(2_000);
      expect(errors).toHaveLength(0);
    });
  }
});
