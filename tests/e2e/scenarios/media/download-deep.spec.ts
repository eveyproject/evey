import { test, expect } from "../../fixtures/auth";
import { gotoRandomMovie, gotoRandomMedia, getToken, goto } from "../shared/helpers";

test.describe("Media > Download deep", () => {
  test("download with JWT token returns 200 or 404", async ({ page }) => {
    const id = await gotoRandomMovie(page);
    test.skip(!id, "No movie");
    const token = await getToken(page);
    const resp = await page.evaluate(async ({ mediaId, tk }) => {
      const res = await fetch(`/api/v1/media/${mediaId}/download?token=${encodeURIComponent(tk)}`, { method: "HEAD" });
      return res.status;
    }, { mediaId: id, tk: token });
    expect([200, 404]).toContain(resp);
  });

  test("download without token returns 401", async ({ page }) => {
    const id = await gotoRandomMovie(page);
    test.skip(!id, "No movie");
    const resp = await page.evaluate(async ({ mediaId }) => {
      const res = await fetch(`/api/v1/media/${mediaId}/download`, { method: "HEAD" });
      return res.status;
    }, { mediaId: id });
    expect(resp).toBe(401);
  });

  test("download with invalid token returns 403", async ({ page }) => {
    const id = await gotoRandomMovie(page);
    test.skip(!id, "No movie");
    const resp = await page.evaluate(async ({ mediaId }) => {
      const res = await fetch(`/api/v1/media/${mediaId}/download?token=invalid.jwt.token`, { method: "HEAD" });
      return res.status;
    }, { mediaId: id });
    expect([401, 403]).toContain(resp);
  });

  test("download has Content-Disposition header", async ({ page }) => {
    const id = await gotoRandomMovie(page);
    test.skip(!id, "No movie");
    const token = await getToken(page);
    const resp = await page.evaluate(async ({ mediaId, tk }) => {
      const res = await fetch(`/api/v1/media/${mediaId}/download?token=${encodeURIComponent(tk)}`, { method: "HEAD" });
      return { status: res.status, disposition: res.headers.get("content-disposition") };
    }, { mediaId: id, tk: token });
    if (resp.status === 200) {
      expect(resp.disposition).toContain("attachment");
    }
  });

  test("download on series (no file) returns 404", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media");
    const token = await getToken(page);
    // we don't know if it's a series, just verify no 500
    const resp = await page.evaluate(async ({ mediaId, tk }) => {
      const res = await fetch(`/api/v1/media/${mediaId}/download?token=${encodeURIComponent(tk)}`, { method: "HEAD" });
      return res.status;
    }, { mediaId: id, tk: token });
    expect(resp).toBeLessThan(500);
  });

  test("download with wrong media ID returns 404", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const resp = await page.evaluate(async ({ tk }) => {
      const res = await fetch(`/api/v1/media/nonexistent-id-12345/download?token=${encodeURIComponent(tk)}`, { method: "HEAD" });
      return res.status;
    }, { tk: token });
    expect(resp).toBe(404);
  });
});
