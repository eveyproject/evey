import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";

const auth = vi.hoisted(() => ({ profileId: "kid" as string | null }));
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    vi.fn((selector: (s: { profileId: string | null }) => unknown) => selector({ profileId: auth.profileId })),
    { getState: () => ({ profileId: auth.profileId }) },
  ),
}));

const api = vi.hoisted(() => ({
  listeners: new Set<(i: { nextOpen?: string }) => void>(),
  getProfileSchedule: vi.fn(),
  unlockProfileSchedule: vi.fn(),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  onProfileClosed: (fn: (i: { nextOpen?: string }) => void) => {
    api.listeners.add(fn);
    return () => api.listeners.delete(fn);
  },
  getProfileSchedule: (...a: unknown[]) => api.getProfileSchedule(...a),
  unlockProfileSchedule: (...a: unknown[]) => api.unlockProfileSchedule(...a),
}));

import { useProfileLock } from "./useProfileLock";

const state = (open: boolean, nextOpen?: string) => ({ access: { open, overridden: false, nextOpen } });

beforeEach(() => {
  auth.profileId = "kid";
  api.listeners.clear();
  api.getProfileSchedule.mockReset();
  api.unlockProfileSchedule.mockReset();
});

// Any refused request locks the app; the server's answer, or a parent's PIN,
// unlocks it, and the app is told to refetch what failed meanwhile.
describe("useProfileLock", () => {
  it("locks on the signal and reopens when the server says open", async () => {
    const onReopen = vi.fn();
    const { result } = renderHook(() => useProfileLock(onReopen));
    expect(result.current.closed).toBe(false);

    act(() => api.listeners.forEach((fn) => fn({ nextOpen: "2026-10-05T16:00:00Z" })));
    expect(result.current.closed).toBe(true);
    expect(result.current.nextOpen).toBe("2026-10-05T16:00:00Z");

    api.getProfileSchedule.mockResolvedValue(state(true));
    act(() => result.current.recheck());
    await waitFor(() => expect(result.current.closed).toBe(false));
    expect(onReopen).toHaveBeenCalledTimes(1);
  });

  it("maps the unlock answers", async () => {
    const { result } = renderHook(() => useProfileLock());
    act(() => api.listeners.forEach((fn) => fn({})));

    api.unlockProfileSchedule.mockRejectedValueOnce(Object.assign(new Error("x"), { httpStatus: 403 }));
    expect(await result.current.unlock("0000")).toBe("wrong_pin");
    api.unlockProfileSchedule.mockRejectedValueOnce(Object.assign(new Error("x"), { httpStatus: 409 }));
    expect(await result.current.unlock("0000")).toBe("no_parent_pin");
    api.unlockProfileSchedule.mockRejectedValueOnce(Object.assign(new Error("x"), { httpStatus: 429 }));
    expect(await result.current.unlock("0000")).toBe("rate_limited");

    api.unlockProfileSchedule.mockResolvedValueOnce(state(true));
    let res: string | null = "pending";
    await act(async () => {
      res = await result.current.unlock("1234");
    });
    expect(res).toBeNull();
    expect(result.current.closed).toBe(false);
  });

  // The device switched to the adult profile while a kids request was in
  // flight: neither the late refusal nor the late answer locks the adult.
  it("ignores a refusal or an answer for a profile the device has left", async () => {
    const { result } = renderHook(() => useProfileLock());
    act(() => api.listeners.forEach((fn) => fn({ profileId: "someone-else" })));
    expect(result.current.closed).toBe(false);

    let resolve!: (v: unknown) => void;
    api.getProfileSchedule.mockReturnValue(new Promise((r) => (resolve = r)));
    act(() => result.current.recheck());
    auth.profileId = "adult";
    await act(async () => resolve(state(false, "2026-10-05T16:00:00Z")));
    expect(result.current.closed).toBe(false);
  });
});
