import { test, expect } from "./fixtures/auth";
import { test as base } from "@playwright/test";

test.describe("Quick Connect — settings tab", () => {
  test("settings page shows Quick Connect tab", async ({ page }) => {
    await page.goto("/profile/settings");

    const main = page.getByRole("main");
    const qcTab = main.getByRole("button", { name: "Quick Connect" });
    await expect(qcTab).toBeVisible({ timeout: 15_000 });
  });

  test("Quick Connect tab shows code input and authorize button", async ({ page }) => {
    await page.goto("/profile/settings");

    const main = page.getByRole("main");
    const qcTab = main.getByRole("button", { name: "Quick Connect" });
    await expect(qcTab).toBeVisible({ timeout: 15_000 });
    await qcTab.click();

    const input = page.getByPlaceholder("A1B2C3");
    await expect(input).toBeVisible({ timeout: 5_000 });

    const authorizeBtn = page.getByRole("button", { name: /Authorize/i });
    await expect(authorizeBtn).toBeVisible();
    await expect(authorizeBtn).toBeDisabled();
  });

  test("authorize button enables when code is entered", async ({ page }) => {
    await page.goto("/profile/settings");

    const main = page.getByRole("main");
    const qcTab = main.getByRole("button", { name: "Quick Connect" });
    await expect(qcTab).toBeVisible({ timeout: 15_000 });
    await qcTab.click();

    const input = page.getByPlaceholder("A1B2C3");
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill("AB12CD");

    const authorizeBtn = page.getByRole("button", { name: /Authorize/i });
    await expect(authorizeBtn).toBeEnabled();
  });

  test("submitting invalid code shows error toast", async ({ page }) => {
    await page.goto("/profile/settings");

    const main = page.getByRole("main");
    const qcTab = main.getByRole("button", { name: "Quick Connect" });
    await expect(qcTab).toBeVisible({ timeout: 15_000 });
    await qcTab.click();

    const input = page.getByPlaceholder("A1B2C3");
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill("XXXXXX");

    const authorizeBtn = page.getByRole("button", { name: /Authorize/i });
    await authorizeBtn.click();

    const toast = page.locator('[role="alert"]').first();
    await expect(toast).toBeVisible({ timeout: 10_000 });
  });
});

test.describe("Quick Connect — standalone page", () => {
  test("quickconnect route loads authorize page", async ({ page }) => {
    await page.goto("/quickconnect");

    const input = page.getByPlaceholder("A1B2C3");
    await expect(input).toBeVisible({ timeout: 15_000 });

    const authorizeBtn = page.getByRole("button", { name: /Authorize/i });
    await expect(authorizeBtn).toBeVisible();
  });

  test("quickconnect route with code param pre-fills input", async ({ page }) => {
    await page.goto("/quickconnect?code=qc-code-aabbccdd11223344");

    const input = page.getByPlaceholder("A1B2C3");
    await expect(input).toBeVisible({ timeout: 15_000 });
    await expect(input).toHaveValue("223344");
  });
});

base.describe("Quick Connect — login page", () => {
  base("login page shows QR code with 120s expiry", async ({ page }) => {
    await page.goto("/login");

    const qcLabel = page.getByText("Quick Connect");
    await expect(qcLabel).toBeVisible({ timeout: 15_000 });

    const expiry = page.getByText(/Expires in \d+s/);
    await expect(expiry).toBeVisible({ timeout: 15_000 });
    const text = await expiry.textContent();
    const seconds = parseInt(text!.match(/(\d+)/)?.[1] ?? "0");
    expect(seconds).toBeGreaterThan(60);
    expect(seconds).toBeLessThanOrEqual(120);
  });
});
