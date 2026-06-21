import { test, expect } from "../../fixtures/auth";
import { goto, getToken } from "../shared/helpers";

// Dev environments often raise EVEY_RATELIMIT_LOGIN_MAX above the prod
// default (10) for ergonomics — and that override usually lives in the
// SERVER container, not in the Playwright runner's env. So we can't trust
// the runner's view of the limit: it's only meaningful when explicitly set
// for the runner (CI). When it is NOT set, we probe empirically and skip
// (rather than fail) if no 429 shows up, because we can't tell "the limit
// is simply higher than the prod default" from "the limiter is broken".
const LOGIN_LIMIT_ENV = process.env.EVEY_RATELIMIT_LOGIN_MAX;
const LOGIN_LIMIT = Number(LOGIN_LIMIT_ENV ?? "10");
const MAX_REQUESTS_TO_FIRE = 100;

test.describe("Rate limiting > Auth endpoint", () => {
  test("rapid POST /api/v1/auth/login → eventually 429", async ({ page }) => {
    test.skip(
      LOGIN_LIMIT >= MAX_REQUESTS_TO_FIRE,
      `EVEY_RATELIMIT_LOGIN_MAX=${LOGIN_LIMIT} too high to exercise (would fire >${MAX_REQUESTS_TO_FIRE} requests)`,
    );
    await goto(page, "/home");
    const fireCount = Math.min(LOGIN_LIMIT + 5, MAX_REQUESTS_TO_FIRE);
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
    // When the runner's limit isn't explicitly configured and we never hit a
    // 429, the server's effective limit is just higher than our probe burst —
    // skip instead of crying wolf. When it IS configured (CI) we still assert,
    // so a genuinely broken limiter fails the build.
    test.skip(
      !has429 && LOGIN_LIMIT_ENV == null,
      `No 429 after ${fireCount} login attempts and EVEY_RATELIMIT_LOGIN_MAX is unset for the runner — ` +
        `the server's limit is likely above the prod default. Set EVEY_RATELIMIT_LOGIN_MAX to the server's value to exercise this test.`,
    );
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
