import { test, expect } from "../../fixtures/auth";
import { goto, getToken } from "../shared/helpers";

// Dev environments often raise EVEY_RATELIMIT_LOGIN_MAX above the prod
// default (10) for ergonomics. Pass the same value to Playwright so this
// test can either fire enough requests to exhaust it or skip when the
// limit is too high to exercise without spamming the server.
const LOGIN_LIMIT = Number(process.env.EVEY_RATELIMIT_LOGIN_MAX ?? "10");
const MAX_REQUESTS_TO_FIRE = 100;

test.describe("Rate limiting > Auth endpoint", () => {
  test("rapid POST /api/v1/auth/login → eventually 429", async ({ page }) => {
    test.skip(
      LOGIN_LIMIT >= MAX_REQUESTS_TO_FIRE,
      `EVEY_RATELIMIT_LOGIN_MAX=${LOGIN_LIMIT} too high to exercise (would fire >${MAX_REQUESTS_TO_FIRE} requests)`,
    );
    await goto(page, "/home");
    const fireCount = LOGIN_LIMIT + 5;
    const statuses = await page.evaluate(async (count) => {
      const results: number[] = [];
      const promises = Array.from({ length: count }, () =>
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
    }, fireCount);
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
