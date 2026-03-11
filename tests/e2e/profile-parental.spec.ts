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
    expect(body).toHaveProperty("maxRating");
    expect(body).toHaveProperty("blockedTags");
    expect(body).toHaveProperty("hasPin");
    expect(body).toHaveProperty("enabled");
  });

  test("set parental PIN and rating limit", async ({ request, token }) => {
    const profileId = await getProfileId(request, token);
    expect(profileId).toBeTruthy();

    const res = await request.put(`/api/v1/profiles/${profileId}/parental`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: {
        pin: "1234",
        maxRating: "PG-13",
        enabled: true,
      },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.ok).toBe(true);

    // Verify settings persisted
    const getRes = await request.get(`/api/v1/profiles/${profileId}/parental`, {
      headers: authHeaders(token),
    });
    expect(getRes.ok()).toBeTruthy();
    const settings = await getRes.json();
    expect(settings.maxRating).toBe("PG-13");
    expect(settings.hasPin).toBe(true);
    expect(settings.enabled).toBe(true);
  });

  test("verify PIN — correct and incorrect", async ({ request, token }) => {
    const profileId = await getProfileId(request, token);

    // Correct PIN
    const validRes = await request.post(`/api/v1/profiles/${profileId}/parental/verify-pin`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { pin: "1234" },
    });
    expect(validRes.ok()).toBeTruthy();
    expect((await validRes.json()).valid).toBe(true);

    // Wrong PIN
    const invalidRes = await request.post(`/api/v1/profiles/${profileId}/parental/verify-pin`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { pin: "0000" },
    });
    expect(invalidRes.ok()).toBeTruthy();
    expect((await invalidRes.json()).valid).toBe(false);
  });

  test("update rating requires current PIN when one is set", async ({ request, token }) => {
    const profileId = await getProfileId(request, token);

    // Without current PIN — should fail 403
    const failRes = await request.put(`/api/v1/profiles/${profileId}/parental`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { maxRating: "R", currentPin: "" },
    });
    expect(failRes.status()).toBe(403);

    // With correct current PIN — should succeed
    const okRes = await request.put(`/api/v1/profiles/${profileId}/parental`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { maxRating: "R", currentPin: "1234" },
    });
    expect(okRes.ok()).toBeTruthy();

    // Verify
    const getRes = await request.get(`/api/v1/profiles/${profileId}/parental`, {
      headers: authHeaders(token),
    });
    expect((await getRes.json()).maxRating).toBe("R");
  });

  test("clear PIN and disable parental controls", async ({ request, token }) => {
    const profileId = await getProfileId(request, token);

    const res = await request.put(`/api/v1/profiles/${profileId}/parental`, {
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      data: { pin: "", maxRating: "", enabled: false, currentPin: "1234" },
    });
    expect(res.ok()).toBeTruthy();

    // Verify
    const getRes = await request.get(`/api/v1/profiles/${profileId}/parental`, {
      headers: authHeaders(token),
    });
    const settings = await getRes.json();
    expect(settings.hasPin).toBe(false);
    expect(settings.enabled).toBe(false);
  });
});
