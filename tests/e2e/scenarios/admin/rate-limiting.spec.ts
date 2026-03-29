import { test, expect } from "../../fixtures/auth";
import { goto, getToken } from "../shared/helpers";

test.describe("Rate limiting > Auth endpoint", () => {
  test("15 rapid POST /api/v1/auth/login → 429 before 15th", async ({ page }) => {
    await goto(page, "/home");
    const statuses = await page.evaluate(async () => {
      const results: number[] = [];
      const promises = Array.from({ length: 15 }, () =>
        fetch("/api/v1/auth/login", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username: "fakeuser", password: "fakepass" }),
        }).then((r) => r.status)
      );
      const settled = await Promise.allSettled(promises);
      for (const s of settled) {
        results.push(s.status === "fulfilled" ? s.value : 0);
      }
      return results;
    });
    const has429 = statuses.some((s) => s === 429);
    expect(has429).toBe(true);
  });
});

test.describe("Rate limiting > Normal GET not throttled", () => {
  test("GET /api/v1/media within normal rate → no 429", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const statuses = await page.evaluate(async ({ tk }) => {
      const results: number[] = [];
      for (let i = 0; i < 5; i++) {
        const res = await fetch("/api/v1/media?pagination.pageSize=1", {
          headers: { Authorization: `Bearer ${tk}` },
        });
        results.push(res.status);
      }
      return results;
    }, { tk: token });
    const has429 = statuses.some((s) => s === 429);
    expect(has429).toBe(false);
  });
});
