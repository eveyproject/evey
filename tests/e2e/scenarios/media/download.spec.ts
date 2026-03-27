import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, getToken } from "../shared/helpers";

test.describe("Media > Download", () => {
  test("download API returns 200 with token", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const token = await getToken(page);
    const resp = await page.evaluate(async ({ mediaId, tk }) => {
      const res = await fetch(`/api/v1/media/${mediaId}/download?token=${encodeURIComponent(tk)}`, { method: "HEAD" });
      return { status: res.status, contentType: res.headers.get("content-type") };
    }, { mediaId: id, tk: token });
    // 200 = file exists, 404 = no file path (series without file)
    expect([200, 404]).toContain(resp.status);
  });

  test("download without token returns 401", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const resp = await page.evaluate(async ({ mediaId }) => {
      const res = await fetch(`/api/v1/media/${mediaId}/download`, { method: "HEAD" });
      return res.status;
    }, { mediaId: id });
    expect(resp).toBe(401);
  });
});
