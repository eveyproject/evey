import { describe, expect, it, vi } from "vitest";
import { useAuthStore } from "@evey/store";
import { leaveOnlyWhenSessionGone } from "./fetcherSetup";

// A 401 on one call while a live token is held must not sign the viewer out
// of a film; a refused refresh (which clears the token) does send them to login.
describe("leaveOnlyWhenSessionGone", () => {
  it("stays while a live token is held", () => {
    useAuthStore.setState({ accessToken: "live" });
    const go = vi.fn();
    leaveOnlyWhenSessionGone(go)();
    expect(go).not.toHaveBeenCalled();
  });

  it("leaves for login once the session is gone", () => {
    useAuthStore.setState({ accessToken: null });
    const go = vi.fn();
    leaveOnlyWhenSessionGone(go)();
    expect(go).toHaveBeenCalledOnce();
  });
});
