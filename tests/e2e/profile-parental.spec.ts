import { test, expect, authHeaders } from "./fixtures/api";

async function getProfileId(
  request: import("@playwright/test").APIRequestContext,
  token: string,
): Promise<string> {
  const userRes = await request.get("/api/v1/users/me", {
    headers: authHeaders(token),
  });
  const user = await userRes.json();
  const profilesRes = await request.get(`/api/v1/users/${user.id}/profiles`, {
    headers: authHeaders(token),
  });
  const body = await profilesRes.json();
  const profiles: { id: string; isDefault?: boolean }[] = body.profiles ?? [];
  const profile = profiles.find((p) => p.isDefault) ?? profiles[0];
  return profile?.id ?? "";
}

// The parental schema migrated from a string-rating ("PG-13", "R") to an
// integer max-age (years). Tests below target the current schema as wired in
// internal/adapters/driving/http/auth_handler.go (parentalHandler).
//
// First-time PIN set requires the account password (defense against a stolen
// JWT silently enabling parental + setting an attacker-picked PIN). We need
// EVEY_PASS to drive the full CRUD; otherwise the PIN-related tests skip.
//
// PIN-handling endpoints share a per-profile token bucket with burst 3 and
// refill 0.6/min (cmd/evey/http.go: ratelimit.NewWithBurst(3, 0.6)). A single
// failed local run easily exhausts it, and the bucket only refills 1 token
// per ~100s. To keep the suite usable on a long-lived dev profile, the
// PIN-mutating tests opt-in via E2E_PARENTAL_PIN=1 (CI sets it on a fresh
// container). The read-only check still runs everywhere.
const ACCOUNT_PASSWORD = process.env.EVEY_PASS;
const RUN_PIN_TESTS = process.env.E2E_PARENTAL_PIN === "1";

test.describe("Profile parental controls", () => {
  test.describe.configure({ mode: "serial" });

  test("get parental settings returns expected fields", async ({ request, token }) => {
    const profileId = await getProfileId(request, token);
    expect(profileId).toBeTruthy();

    const res = await request.get(`/api/v1/profiles/${profileId}/parental`, {
      headers: authHeaders(token),
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body).toHaveProperty("maxAge");
    expect(body).toHaveProperty("blockedTags");
    expect(body).toHaveProperty("hasPin");
    expect(body).toHaveProperty("enabled");
  });

  test("set parental PIN and max age", async ({ request, token }) => {
    test.skip(!RUN_PIN_TESTS, "PIN-mutating tests need a fresh per-profile rate-limit bucket; opt-in with E2E_PARENTAL_PIN=1");
    test.skip(!ACCOUNT_PASSWORD, "EVEY_PASS not provided; cannot satisfy first-time PIN guard");
    const profileId = await getProfileId(request, token);
    expect(profileId).toBeTruthy();

    const res = await request.put(`/api/v1/profiles/${profileId}/parental`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { pin: "1234", maxAge: 13, enabled: true, currentPassword: ACCOUNT_PASSWORD },
    });
    expect(res.ok()).toBeTruthy();
    expect((await res.json()).ok).toBe(true);

    const getRes = await request.get(`/api/v1/profiles/${profileId}/parental`, {
      headers: authHeaders(token),
    });
    expect(getRes.ok()).toBeTruthy();
    const settings = await getRes.json();
    expect(settings.maxAge).toBe(13);
    expect(settings.hasPin).toBe(true);
    expect(settings.enabled).toBe(true);
  });

  test("verify PIN — correct and incorrect", async ({ request, token }) => {
    test.skip(!RUN_PIN_TESTS, "PIN-mutating tests need a fresh per-profile rate-limit bucket; opt-in with E2E_PARENTAL_PIN=1");
    const profileId = await getProfileId(request, token);

    const validRes = await request.post(`/api/v1/profiles/${profileId}/parental/verify-pin`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { pin: "1234" },
    });
    expect(validRes.ok()).toBeTruthy();
    expect((await validRes.json()).valid).toBe(true);

    const invalidRes = await request.post(`/api/v1/profiles/${profileId}/parental/verify-pin`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { pin: "0000" },
    });
    expect(invalidRes.ok()).toBeTruthy();
    expect((await invalidRes.json()).valid).toBe(false);
  });

  test("update max age requires current PIN when one is set", async ({ request, token }) => {
    test.skip(!RUN_PIN_TESTS, "PIN-mutating tests need a fresh per-profile rate-limit bucket; opt-in with E2E_PARENTAL_PIN=1");
    const profileId = await getProfileId(request, token);

    // Without current PIN — should be rejected.
    const failRes = await request.put(`/api/v1/profiles/${profileId}/parental`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { maxAge: 17, currentPin: "" },
    });
    expect([400, 401, 403]).toContain(failRes.status());

    // With correct current PIN — should succeed.
    const okRes = await request.put(`/api/v1/profiles/${profileId}/parental`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { maxAge: 17, currentPin: "1234" },
    });
    expect(okRes.ok()).toBeTruthy();

    const getRes = await request.get(`/api/v1/profiles/${profileId}/parental`, {
      headers: authHeaders(token),
    });
    expect((await getRes.json()).maxAge).toBe(17);
  });

  test("clear PIN and disable parental controls", async ({ request, token }) => {
    test.skip(!RUN_PIN_TESTS, "PIN-mutating tests need a fresh per-profile rate-limit bucket; opt-in with E2E_PARENTAL_PIN=1");
    const profileId = await getProfileId(request, token);

    const res = await request.put(`/api/v1/profiles/${profileId}/parental`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { pin: "", unsetMaxAge: true, enabled: false, currentPin: "1234" },
    });
    expect(res.ok()).toBeTruthy();

    const getRes = await request.get(`/api/v1/profiles/${profileId}/parental`, {
      headers: authHeaders(token),
    });
    const settings = await getRes.json();
    expect(settings.hasPin).toBe(false);
    expect(settings.enabled).toBe(false);
  });
});
