import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "./auth";

// Build a minimal JWT with the given payload claims (header+signature are fake stubs).
function makeJwt(claims: Record<string, unknown>): string {
  const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const payload = btoa(JSON.stringify(claims));
  return `${header}.${payload}.stub-sig`;
}

function resetStore() {
  useAuthStore.setState({
    tokensByServer: {},
    accessToken: null,
    profileId: null,
    isAdmin: false,
    // Default: no active server - tests that need one override _getActiveServerId.
    _getActiveServerId: () => null,
  });
}

beforeEach(resetStore);

// ─── hydrateFromPersisted (cross-tab sync) ──────────────────────────────────────

describe("hydrateFromPersisted", () => {
  it("restores impersonationOrigin so a cross-tab exitImpersonation works", () => {
    const originToken = makeJwt({ is_admin: true });
    const impToken = makeJwt({ imp_by: "usr-admin", username: "Target", profile_id: "prf-t", is_admin: true });
    // Another tab impersonated: persisted blob carries the impersonation token
    // AND the admin's own origin token.
    useAuthStore.getState().hydrateFromPersisted({
      accessToken: impToken,
      impersonatedBy: "usr-admin",
      impersonatedUsername: "Target",
      impersonationOrigin: originToken,
      isAdmin: true,
      profileId: "prf-t",
    });
    let s = useAuthStore.getState();
    expect(s.accessToken).toBe(impToken);
    expect(s.impersonatedBy).toBe("usr-admin");
    expect(s.impersonationOrigin).toBe(originToken);

    // Before the fix, setTokens dropped impersonationOrigin → this was a no-op.
    useAuthStore.getState().exitImpersonation();
    s = useAuthStore.getState();
    expect(s.accessToken).toBe(originToken);
    expect(s.impersonationOrigin).toBeNull();
    expect(s.impersonatedBy).toBeNull();
  });

  it("clears stale impersonation fields when the incoming blob has none", () => {
    useAuthStore.setState({ impersonatedBy: "x", impersonatedUsername: "y", impersonationOrigin: "z" });
    useAuthStore.getState().hydrateFromPersisted({ accessToken: makeJwt({ profile_id: "p" }), profileId: "p" });
    const s = useAuthStore.getState();
    expect(s.impersonatedBy).toBeNull();
    expect(s.impersonatedUsername).toBeNull();
    expect(s.impersonationOrigin).toBeNull();
  });
});

// ─── initial state ────────────────────────────────────────────────────────────

describe("initial state", () => {
  it("has no access token, no profileId, not admin", () => {
    const s = useAuthStore.getState();
    expect(s.accessToken).toBeNull();
    expect(s.profileId).toBeNull();
    expect(s.isAdmin).toBe(false);
    expect(s.tokensByServer).toEqual({});
  });
});

// ─── setTokens ────────────────────────────────────────────────────────────────

describe("setTokens", () => {
  it("stores the access token in shadow fields and tokensByServer", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    const token = makeJwt({ profile_id: "p-1", is_admin: false });

    useAuthStore.getState().setTokens(token);

    const s = useAuthStore.getState();
    expect(s.accessToken).toBe(token);
    expect(s.profileId).toBe("p-1");
    expect(s.isAdmin).toBe(false);
    expect(s.tokensByServer["srv-1"].accessToken).toBe(token);
    expect(s.tokensByServer["srv-1"].profileId).toBe("p-1");
  });

  it("parses isAdmin=true from JWT claim", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    const token = makeJwt({ profile_id: "p-admin", is_admin: true });

    useAuthStore.getState().setTokens(token);

    expect(useAuthStore.getState().isAdmin).toBe(true);
    expect(useAuthStore.getState().tokensByServer["srv-1"].isAdmin).toBe(true);
  });

  it("falls back to null profileId when claim is absent", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    const token = makeJwt({ is_admin: false });

    useAuthStore.getState().setTokens(token);

    expect(useAuthStore.getState().profileId).toBeNull();
    expect(useAuthStore.getState().tokensByServer["srv-1"].profileId).toBeNull();
  });

  it("does not modify tokensByServer when activeServerId is null", () => {
    // _getActiveServerId already returns null (reset above)
    const token = makeJwt({ profile_id: "p-1" });
    useAuthStore.getState().setTokens(token);

    // Shadow fields update (best-effort) but tokensByServer stays empty
    expect(useAuthStore.getState().tokensByServer).toEqual({});
  });

  it("scopes tokens to the active server only", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    const tokenA = makeJwt({ profile_id: "p-a", is_admin: false });
    useAuthStore.getState().setTokens(tokenA);

    // Switch active server and set a different token
    useAuthStore.setState({ _getActiveServerId: () => "srv-2" });
    const tokenB = makeJwt({ profile_id: "p-b", is_admin: true });
    useAuthStore.getState().setTokens(tokenB);

    const s = useAuthStore.getState();
    expect(s.tokensByServer["srv-1"].accessToken).toBe(tokenA);
    expect(s.tokensByServer["srv-1"].profileId).toBe("p-a");
    expect(s.tokensByServer["srv-2"].accessToken).toBe(tokenB);
    expect(s.tokensByServer["srv-2"].profileId).toBe("p-b");
  });
});

// ─── setProfile ───────────────────────────────────────────────────────────────

describe("setProfile", () => {
  it("updates profileId shadow field and tokensByServer entry", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    const token = makeJwt({ profile_id: "p-old" });
    useAuthStore.getState().setTokens(token);

    useAuthStore.getState().setProfile("p-new");

    expect(useAuthStore.getState().profileId).toBe("p-new");
    expect(useAuthStore.getState().tokensByServer["srv-1"].profileId).toBe("p-new");
  });

  it("does not affect the accessToken stored for the server", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    const token = makeJwt({ profile_id: "p-1" });
    useAuthStore.getState().setTokens(token);

    useAuthStore.getState().setProfile("p-switched");

    expect(useAuthStore.getState().tokensByServer["srv-1"].accessToken).toBe(token);
  });
});

// ─── logout (current-server isolation) ───────────────────────────────────────

describe("logout", () => {
  it("clears shadow fields", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    useAuthStore.getState().setTokens(makeJwt({ profile_id: "p-1", is_admin: true }));

    useAuthStore.getState().logout();

    const s = useAuthStore.getState();
    expect(s.accessToken).toBeNull();
    expect(s.profileId).toBeNull();
    expect(s.isAdmin).toBe(false);
  });

  it("clears only the active server in tokensByServer", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    useAuthStore.getState().setTokens(makeJwt({ profile_id: "p-1" }));

    useAuthStore.setState({ _getActiveServerId: () => "srv-2" });
    const tokenB = makeJwt({ profile_id: "p-2" });
    useAuthStore.getState().setTokens(tokenB);

    // Log out from srv-2 (currently active)
    useAuthStore.getState().logout();

    const s = useAuthStore.getState();
    // srv-2 cleared
    expect(s.tokensByServer["srv-2"].accessToken).toBeNull();
    expect(s.tokensByServer["srv-2"].profileId).toBeNull();
    // srv-1 untouched - multi-server isolation invariant
    expect(s.tokensByServer["srv-1"].accessToken).toBe(
      useAuthStore.getState().tokensByServer["srv-1"].accessToken
    );
    expect(s.tokensByServer["srv-1"].profileId).toBe("p-1");
  });

  it("preserves OTHER servers tokens when logging out of current server", () => {
    // Seed two servers
    const tokenA = makeJwt({ profile_id: "p-a", is_admin: false });
    const tokenB = makeJwt({ profile_id: "p-b", is_admin: true });

    useAuthStore.setState({ _getActiveServerId: () => "srv-a" });
    useAuthStore.getState().setTokens(tokenA);
    useAuthStore.setState({ _getActiveServerId: () => "srv-b" });
    useAuthStore.getState().setTokens(tokenB);

    // Logout while active server is srv-b
    useAuthStore.getState().logout();

    // srv-a must be completely intact
    const remaining = useAuthStore.getState().tokensByServer["srv-a"];
    expect(remaining.accessToken).toBe(tokenA);
    expect(remaining.profileId).toBe("p-a");
    expect(remaining.isAdmin).toBe(false);
    // srv-b cleared
    expect(useAuthStore.getState().tokensByServer["srv-b"].accessToken).toBeNull();
  });

  it("no-ops tokensByServer when activeServerId is null", () => {
    // Pre-populate a server without using _getActiveServerId
    useAuthStore.setState({
      tokensByServer: { "srv-x": { accessToken: "t", profileId: "p", isAdmin: false } },
      _getActiveServerId: () => null,
    });

    useAuthStore.getState().logout();

    // tokensByServer unchanged (no active server to clear)
    expect(useAuthStore.getState().tokensByServer["srv-x"].accessToken).toBe("t");
  });
});

// ─── logoutAll ────────────────────────────────────────────────────────────────

describe("logoutAll", () => {
  it("clears shadow fields", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    useAuthStore.getState().setTokens(makeJwt({ profile_id: "p-1", is_admin: true }));

    useAuthStore.getState().logoutAll();

    const s = useAuthStore.getState();
    expect(s.accessToken).toBeNull();
    expect(s.profileId).toBeNull();
    expect(s.isAdmin).toBe(false);
  });

  it("empties tokensByServer entirely", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    useAuthStore.getState().setTokens(makeJwt({ profile_id: "p-1" }));
    useAuthStore.setState({ _getActiveServerId: () => "srv-2" });
    useAuthStore.getState().setTokens(makeJwt({ profile_id: "p-2" }));

    useAuthStore.getState().logoutAll();

    expect(useAuthStore.getState().tokensByServer).toEqual({});
  });

  it("removes ALL servers, not just the active one", () => {
    const tokenA = makeJwt({ profile_id: "p-a" });
    const tokenB = makeJwt({ profile_id: "p-b" });
    const tokenC = makeJwt({ profile_id: "p-c" });

    useAuthStore.setState({ _getActiveServerId: () => "srv-a" });
    useAuthStore.getState().setTokens(tokenA);
    useAuthStore.setState({ _getActiveServerId: () => "srv-b" });
    useAuthStore.getState().setTokens(tokenB);
    useAuthStore.setState({ _getActiveServerId: () => "srv-c" });
    useAuthStore.getState().setTokens(tokenC);

    useAuthStore.getState().logoutAll();

    expect(Object.keys(useAuthStore.getState().tokensByServer)).toHaveLength(0);
  });
});

// ─── syncFromServer ───────────────────────────────────────────────────────────

describe("syncFromServer", () => {
  it("loads shadow fields from tokensByServer for the given id", () => {
    const token = makeJwt({ profile_id: "p-1", is_admin: true });
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    useAuthStore.getState().setTokens(token);

    // Switch shadow to empty state, then sync back from srv-1
    useAuthStore.setState({ accessToken: null, profileId: null, isAdmin: false });
    useAuthStore.getState().syncFromServer("srv-1");

    const s = useAuthStore.getState();
    expect(s.accessToken).toBe(token);
    expect(s.profileId).toBe("p-1");
    expect(s.isAdmin).toBe(true);
  });

  it("resets shadow fields when syncing an unknown server id", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    useAuthStore.getState().setTokens(makeJwt({ profile_id: "p-1", is_admin: true }));

    useAuthStore.getState().syncFromServer("srv-unknown");

    const s = useAuthStore.getState();
    expect(s.accessToken).toBeNull();
    expect(s.profileId).toBeNull();
    expect(s.isAdmin).toBe(false);
  });

  it("resets shadow fields when called with null", () => {
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    useAuthStore.getState().setTokens(makeJwt({ profile_id: "p-1", is_admin: true }));

    useAuthStore.getState().syncFromServer(null);

    const s = useAuthStore.getState();
    expect(s.accessToken).toBeNull();
    expect(s.profileId).toBeNull();
    expect(s.isAdmin).toBe(false);
  });

  it("does not alter tokensByServer", () => {
    const token = makeJwt({ profile_id: "p-1" });
    useAuthStore.setState({ _getActiveServerId: () => "srv-1" });
    useAuthStore.getState().setTokens(token);

    useAuthStore.getState().syncFromServer("srv-1");

    expect(useAuthStore.getState().tokensByServer["srv-1"].accessToken).toBe(token);
  });
});
