import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "@evey/store";

const ADMIN_TOKEN =
  "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9." +
  btoa(JSON.stringify({ sub: "user1", is_admin: true, profile_id: "p-admin" })) +
  ".sig";

const USER_TOKEN =
  "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9." +
  btoa(JSON.stringify({ sub: "user2", is_admin: false, profile_id: "p-user", locale: "fr-FR" })) +
  ".sig";

const MALFORMED_TOKEN = "not.a.jwt";

function reset() {
  useAuthStore.setState({ accessToken: null, profileId: null, isAdmin: false });
}

describe("useAuthStore", () => {
  beforeEach(reset);

  it("starts unauthenticated", () => {
    const s = useAuthStore.getState();
    expect(s.accessToken).toBeNull();
    expect(s.profileId).toBeNull();
    expect(s.isAdmin).toBe(false);
  });

  describe("setTokens", () => {
    it("stores access token", () => {
      useAuthStore.getState().setTokens(ADMIN_TOKEN);
      expect(useAuthStore.getState().accessToken).toBe(ADMIN_TOKEN);
    });

    it("extracts isAdmin=true from claims", () => {
      useAuthStore.getState().setTokens(ADMIN_TOKEN);
      expect(useAuthStore.getState().isAdmin).toBe(true);
    });

    it("extracts isAdmin=false for non-admin", () => {
      useAuthStore.getState().setTokens(USER_TOKEN);
      expect(useAuthStore.getState().isAdmin).toBe(false);
    });

    it("extracts profile_id from claims", () => {
      useAuthStore.getState().setTokens(ADMIN_TOKEN);
      expect(useAuthStore.getState().profileId).toBe("p-admin");
    });

    it("handles malformed JWT gracefully (no crash, isAdmin=false)", () => {
      useAuthStore.getState().setTokens(MALFORMED_TOKEN);
      const s = useAuthStore.getState();
      expect(s.accessToken).toBe(MALFORMED_TOKEN);
      expect(s.isAdmin).toBe(false);
      expect(s.profileId).toBeNull();
    });
  });

  it("setProfile stores profileId", () => {
    useAuthStore.getState().setProfile("profile-99");
    expect(useAuthStore.getState().profileId).toBe("profile-99");
  });

  describe("logout", () => {
    it("clears access token", () => {
      useAuthStore.getState().setTokens(ADMIN_TOKEN);
      useAuthStore.getState().logout();
      const s = useAuthStore.getState();
      expect(s.accessToken).toBeNull();
    });

    it("resets isAdmin and profileId", () => {
      useAuthStore.getState().setTokens(ADMIN_TOKEN);
      useAuthStore.getState().setProfile("p1");
      useAuthStore.getState().logout();
      const s = useAuthStore.getState();
      expect(s.isAdmin).toBe(false);
      expect(s.profileId).toBeNull();
    });
  });
});
