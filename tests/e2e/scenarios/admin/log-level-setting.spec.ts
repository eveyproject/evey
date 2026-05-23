import { test, expect } from "../../fixtures/auth";
import { goto } from "../shared/helpers";

// The log level is an env default editable from the admin Server Config panel
// (live: the running logger's level is updated on save). This verifies the
// setting renders, saves, and persists across reload.
test.describe("Admin > Server Config > Log level", () => {
  test("log level select persists across reload", async ({ page }) => {
    await goto(page, "/admin/server-config");
    const select = page.getByTestId("log-level-select");
    await expect(select).toBeVisible({ timeout: 10_000 });

    await select.selectOption("warn");
    await page.getByTestId("server-config-save-btn").click();

    await goto(page, "/admin/server-config");
    await expect(page.getByTestId("log-level-select")).toHaveValue("warn", { timeout: 10_000 });

    // Reset back to env default so we don't leave the dev server at warn.
    await page.getByTestId("log-level-select").selectOption("info");
    await page.getByTestId("server-config-save-btn").click();
  });
});
