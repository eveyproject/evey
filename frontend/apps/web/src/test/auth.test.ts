import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "@evey/store";

// JWT with payload {"sub":"user1","is_admin":true}
const ADMIN_TOKEN =
  "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9." +
  btoa(JSON.stringify({ sub: "user1", is_admin: true })) +
  ".sig";

// JWT with payload {"sub":"user2","is_admin":false}
const USER_TOKEN =
  "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9." +
  btoa(JSON.stringify({ sub: "user2", is_admin: false })) +
  ".sig";

function resetStore() {
  useAuthStore.setState({
    accessToken: null,
    profileId: null,
    isAdmin: false,
  });
}

describe("useAuthStore", () => {
  beforeEach(resetStore);

  it("starts unauthenticated", () => {
    const s = useAuthStore.getState();
    expect(s.accessToken).toBeNull();
    expect(s.isAdmin).toBe(false);
  });

  it("setTokens stores access token and extracts isAdmin=true", () => {
    useAuthStore.getState().setTokens(ADMIN_TOKEN);
    const s = useAuthStore.getState();
    expect(s.accessToken).toBe(ADMIN_TOKEN);
    expect(s.isAdmin).toBe(true);
  });

  it("setTokens extracts isAdmin=false for non-admin", () => {
    useAuthStore.getState().setTokens(USER_TOKEN);
    expect(useAuthStore.getState().isAdmin).toBe(false);
  });

  it("setProfile stores profileId", () => {
    useAuthStore.getState().setProfile("profile-42");
    expect(useAuthStore.getState().profileId).toBe("profile-42");
  });

  it("logout clears everything", () => {
    useAuthStore.getState().setTokens(ADMIN_TOKEN);
    useAuthStore.getState().setProfile("p1");
    useAuthStore.getState().logout();

    const s = useAuthStore.getState();
    expect(s.accessToken).toBeNull();
    expect(s.profileId).toBeNull();
    expect(s.isAdmin).toBe(false);
  });
});
