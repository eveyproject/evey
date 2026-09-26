import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const mutate = vi.fn();
let played: boolean | undefined;
let onSuccess: (() => void) | undefined;
vi.mock("@evey/api", () => ({
  usePlaybackServiceGetWatchProgress: () => ({ data: played === undefined ? undefined : { played } }),
  usePlaybackServiceMarkAsWatched: (opts: { mutation: { onSuccess: () => void } }) => {
    onSuccess = opts.mutation.onSuccess;
    return { mutate };
  },
}));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { profileId: string | null }) => unknown) => sel({ profileId: "p1" }),
}));
const invalidateQueries = vi.fn();
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries }) }));

import { useAutoMarkWatched } from "./useAutoMarkWatched";

beforeEach(() => {
  vi.clearAllMocks();
  played = undefined;
});

describe("useAutoMarkWatched", () => {
  it("marks once past 90%, then refreshes the watched-sensitive views", () => {
    const { rerender } = renderHook(({ pos }) => useAutoMarkWatched("m1", pos, 100), { initialProps: { pos: 50 } });
    expect(mutate).not.toHaveBeenCalled();
    rerender({ pos: 91 });
    rerender({ pos: 95 });
    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate).toHaveBeenCalledWith({ mediaItemId: "m1", params: { profileId: "p1" } });
    onSuccess?.();
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["/api/v1/media/m1"] });
  });

  it("leaves a title the server already knows as played alone", () => {
    played = true;
    renderHook(() => useAutoMarkWatched("m1", 99, 100));
    expect(mutate).not.toHaveBeenCalled();
  });

  it("arms again for the next title", () => {
    const { rerender } = renderHook(({ id, pos }) => useAutoMarkWatched(id, pos, 100), { initialProps: { id: "m1", pos: 95 } });
    rerender({ id: "m2", pos: 10 });
    rerender({ id: "m2", pos: 92 });
    expect(mutate).toHaveBeenCalledTimes(2);
    expect(mutate).toHaveBeenLastCalledWith({ mediaItemId: "m2", params: { profileId: "p1" } });
  });

  it("ignores an unknown or broken duration", () => {
    renderHook(() => useAutoMarkWatched("m1", 95, 0));
    renderHook(() => useAutoMarkWatched("m1", 95, Number.NaN));
    expect(mutate).not.toHaveBeenCalled();
  });
});
