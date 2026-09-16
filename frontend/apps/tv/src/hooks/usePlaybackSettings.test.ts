import { describe, it, expect, vi } from "vitest";

const captured = vi.hoisted(() => ({ opts: null as any }));
vi.mock("@evey/api", () => ({
  usePlaybackSettingsServiceGetPlaybackSettings: (o: any) => {
    captured.opts = o;
    return { data: undefined };
  },
}));

import { usePlaybackSettings } from "./usePlaybackSettings";

// The server's skip steps drive the D-pad; a field the server leaves out
// (older server, partial answer) falls back to 30 s rather than NaN, and the
// answer is cached for five minutes so every remote press does not refetch.
describe("usePlaybackSettings (tv)", () => {
  it("selects the server's steps with 30 s defaults and a 5 min stale time", () => {
    usePlaybackSettings();
    const { select, staleTime } = captured.opts.query;
    expect(staleTime).toBe(5 * 60 * 1000);
    expect(select({ skipForwardSeconds: 15, skipBackwardSeconds: 10 })).toEqual({ skipForwardSeconds: 15, skipBackwardSeconds: 10 });
    expect(select({ skipForwardSeconds: 45 })).toEqual({ skipForwardSeconds: 45, skipBackwardSeconds: 30 });
    expect(select({})).toEqual({ skipForwardSeconds: 30, skipBackwardSeconds: 30 });
  });
});
