import { test, expect } from "../../fixtures/auth";
import { goto, apiGet, apiDelete, collectErrors } from "../shared/helpers";

test.describe("Admin > Webhooks CRUD (deep)", () => {
  const webhookName = `E2E-Webhook-${Date.now()}`;
  const webhookUrl = "https://httpbin.org/post";
  let webhookId: string | null = null;

  test("create webhook via form", async ({ page }) => {
    await goto(page, "/admin/webhooks");
    await page.locator('[data-testid="add-webhook-btn"]').click();

    const nameInput = page.locator('[data-testid="webhook-name-input"]');
    await expect(nameInput).toBeVisible({ timeout: 3_000 });

    await nameInput.fill(webhookName);
    await page.locator('[data-testid="webhook-url-input"]').fill(webhookUrl);

    await Promise.all([
      page.waitForResponse((r) => r.url().includes("/api/v1/admin/webhooks") && r.request().method() === "POST", { timeout: 5_000 }),
      page.locator('[data-testid="webhook-form-submit"]').click(),
    ]);

    await page.waitForTimeout(1_000);
    const body = await page.locator("body").textContent();
    expect(body).toContain(webhookName);
  });

  test("webhook listed after creation", async ({ page }) => {
    await goto(page, "/admin/webhooks");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).toContain(webhookName);
  });

  test("list webhooks via API", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/admin/webhooks");
    expect(res.status).toBeLessThan(500);
    const webhooks = (res.body as { webhooks?: { id?: string; name?: string }[] })?.webhooks ?? [];
    const found = webhooks.find((w) => w.name === webhookName);
    expect(found).toBeTruthy();
    webhookId = found?.id ?? null;
  });

  test("delete webhook via API", async ({ page }) => {
    test.skip(!webhookId, "No webhook to delete");
    await goto(page, "/home");
    const res = await apiDelete(page, `/api/v1/admin/webhooks/${webhookId}`);
    expect(res.status).toBeLessThan(500);
  });

  test("deleted webhook no longer listed", async ({ page }) => {
    test.skip(!webhookId, "No webhook");
    // The delete went through the API, behind the UI's React Query cache, so a
    // bare load can rehydrate the stale (persisted) list and show the deleted
    // row until the background refetch lands. Wait for that fresh GET to settle
    // before asserting. (Backend delete+list verified correct out-of-band.)
    await goto(page, "/admin/webhooks");
    await page
      .waitForResponse(
        (r) => r.url().includes("/api/v1/admin/webhooks") && r.request().method() === "GET",
        { timeout: 5_000 },
      )
      .catch(() => {});
    await page.waitForTimeout(1_500);
    const body = await page.locator("body").textContent();
    expect(body).not.toContain(webhookName);
  });

  test("no 5xx during full flow", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/webhooks");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
