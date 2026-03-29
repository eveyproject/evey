import { test, expect } from "../../fixtures/auth";
import { goto, apiGet, apiPost, apiDelete, collectErrors } from "../shared/helpers";

test.describe("Admin > Webhooks CRUD (deep)", () => {
  const webhookName = `E2E-Webhook-${Date.now()}`;
  const webhookUrl = "https://httpbin.org/post";
  let webhookId: string | null = null;

  test("create webhook via form", async ({ page }) => {
    await goto(page, "/admin/webhooks");
    await page.locator("button", { hasText: /create|add|new/i }).first().click();

    const nameInput = page.locator('input').first();
    await expect(nameInput).toBeVisible({ timeout: 3_000 });

    await nameInput.fill(webhookName);
    await page.locator('input[type="url"]').fill(webhookUrl);

    await Promise.all([
      page.waitForResponse((r) => r.url().includes("/api/v1/webhooks") && r.request().method() === "POST", { timeout: 5_000 }),
      page.locator("button[type='submit'], button:has-text('Create')").first().click(),
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
    const res = await apiGet(page, "/api/v1/webhooks");
    expect(res.status).toBeLessThan(500);
    const webhooks = (res.body as { webhooks?: { id?: string; name?: string }[] })?.webhooks ?? [];
    const found = webhooks.find((w) => w.name === webhookName);
    expect(found).toBeTruthy();
    webhookId = found?.id ?? null;
  });

  test("delete webhook via API", async ({ page }) => {
    test.skip(!webhookId, "No webhook to delete");
    await goto(page, "/home");
    const res = await apiDelete(page, `/api/v1/webhooks/${webhookId}`);
    expect(res.status).toBeLessThan(500);
  });

  test("deleted webhook no longer listed", async ({ page }) => {
    test.skip(!webhookId, "No webhook");
    await goto(page, "/admin/webhooks");
    await page.waitForTimeout(2_000);
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
