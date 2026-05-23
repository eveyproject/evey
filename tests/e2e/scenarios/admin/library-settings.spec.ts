import { test, expect } from "../../fixtures/auth";
import { goto } from "../shared/helpers";

// Library watcher + periodic scan interval are env defaults now editable from the
// admin Server Config panel (persisted; effect on restart). Verifies the
// periodic-scan-interval persists across reload, then resets it.
test("library scan interval persists across reload", async ({ page }) => {
  await goto(page, "/admin/server-config");
  const scan = page.getByTestId("scan-interval-input");
  await expect(scan).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("library-watcher-toggle")).toBeVisible();

  await scan.fill("10m");
  await page.getByTestId("server-config-save-btn").click();

  await goto(page, "/admin/server-config");
  // Round-trips as Go's canonical duration string ("10m" → "10m0s").
  await expect(page.getByTestId("scan-interval-input")).toHaveValue("10m0s", { timeout: 10_000 });

  // Reset to the env default so the dev box isn't left at 10m.
  await page.getByTestId("scan-interval-input").fill("2m");
  await page.getByTestId("server-config-save-btn").click();
});
