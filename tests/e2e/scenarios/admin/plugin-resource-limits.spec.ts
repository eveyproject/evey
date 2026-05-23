import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Plugins > Resource limits", () => {
  test("section renders with effective caps loaded", async ({ page }) => {
    const errors = await collectErrors(page, async () => {
      await goto(page, "/admin/plugins");
      const card = page.getByTestId("plugin-resource-limits");
      await expect(card).toBeVisible({ timeout: 10_000 });
      // Memory cap is populated from the server (defaults to 256 MiB).
      const mem = page.getByTestId("rl-memory");
      await expect(mem).toBeVisible();
      const val = await mem.inputValue();
      expect(Number(val)).toBeGreaterThan(0);
      // Sandbox select is one of the known modes.
      await expect(page.getByTestId("rl-sandbox")).toBeVisible();
      await expect(page.getByTestId("rl-save")).toBeVisible();
    });
    expect(errors.jsErrors).toEqual([]);
    expect(errors.serverErrors).toEqual([]);
  });

  test("update a cap and persist it", async ({ page }) => {
    await goto(page, "/admin/plugins");
    await expect(page.getByTestId("plugin-resource-limits")).toBeVisible({ timeout: 10_000 });

    const pids = page.getByTestId("rl-pids");
    await pids.fill("48");
    await page.getByTestId("rl-save").click();

    // Reload and confirm the new value round-tripped through the server.
    await goto(page, "/admin/plugins");
    await expect(page.getByTestId("plugin-resource-limits")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByTestId("rl-pids")).toHaveValue("48");
  });
});
