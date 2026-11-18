import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";

let profile: { id: string; preferences?: Record<string, string> } | undefined;
vi.mock("./useActiveProfile", () => ({
  useActiveProfile: () => ({ profile, userId: "u1", profiles: profile ? [profile] : [], isLoading: false }),
}));
const mutateAsync = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceUpdateProfile: () => ({ mutateAsync }),
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
}));
const invalidateQueries = vi.fn();
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries }) }));
vi.mock("@evey/store", () => ({
  autoplayNextEpisode: (p: Record<string, string> | undefined) => p?.["autoplay_next_episode"] !== "false",
  autoplayNextPreferences: (p: Record<string, string> | undefined, on: boolean) => ({ ...p, autoplay_next_episode: String(on) }),
  isDemoMode: () => false,
}));

import { profileLangList, usePlayerProfilePrefs } from "./usePlayerProfilePrefs";
import { DEFAULT_SUBTITLE_STYLE, SUBTITLE_STYLE_PREF_KEY } from "../lib/subtitleStyle";

beforeEach(() => {
  vi.clearAllMocks();
  profile = { id: "p1", preferences: { audio_languages: " fr, en ,", subtitle_languages: "fr" } };
});

describe("usePlayerProfilePrefs", () => {
  it("reads the ordered language lists, blanks dropped", () => {
    const { result } = renderHook(() => usePlayerProfilePrefs());
    expect(result.current.audioLangs).toEqual(["fr", "en"]);
    expect(result.current.subtitleLangs).toEqual(["fr"]);
    expect(profileLangList(undefined)).toEqual([]);
  });

  it("flips auto-play at once, persists it on the running profile and refreshes the list", async () => {
    mutateAsync.mockResolvedValue({});
    const { result } = renderHook(() => usePlayerProfilePrefs());
    expect(result.current.autoplayNext).toBe(true);
    await act(() => result.current.setAutoplayNext(false));
    expect(result.current.autoplayNext).toBe(false);
    expect(mutateAsync).toHaveBeenCalledWith({ id: "p1", data: { preferences: expect.objectContaining({ autoplay_next_episode: "false" }) } });
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["profiles", "u1"] });
  });

  it("falls back to the server value when the save fails", async () => {
    mutateAsync.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => usePlayerProfilePrefs());
    await act(() => result.current.setAutoplayNext(false));
    expect(result.current.autoplayNext).toBe(true);
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("does nothing without a profile", async () => {
    profile = undefined;
    const { result } = renderHook(() => usePlayerProfilePrefs());
    await act(() => result.current.setAutoplayNext(false));
    expect(mutateAsync).not.toHaveBeenCalled();
  });

  describe("subtitle style", () => {
    const bigger = { ...DEFAULT_SUBTITLE_STYLE, fontSize: DEFAULT_SUBTITLE_STYLE.fontSize + 50 };
    const savedStyle = () => JSON.parse(mutateAsync.mock.calls[0][0].data.preferences[SUBTITLE_STYLE_PREF_KEY]);

    beforeEach(() => {
      vi.useFakeTimers();
      mutateAsync.mockResolvedValue({});
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    it("saves a change once the viewer stops", async () => {
      const { result } = renderHook(() => usePlayerProfilePrefs());
      act(() => result.current.saveSubtitleStyle(bigger));
      expect(mutateAsync).not.toHaveBeenCalled();
      await act(() => vi.advanceTimersByTimeAsync(600));
      expect(mutateAsync).toHaveBeenCalledTimes(1);
      expect(savedStyle()).toEqual(bigger);
    });

    it("sends a change still inside the delay when the screen goes away", async () => {
      const { result, unmount } = renderHook(() => usePlayerProfilePrefs());
      act(() => result.current.saveSubtitleStyle(bigger));
      unmount();
      await vi.runAllTimersAsync();
      expect(mutateAsync).toHaveBeenCalledTimes(1);
      expect(savedStyle()).toEqual(bigger);
    });

    it("does not send it twice once the timer already did", async () => {
      const { result, unmount } = renderHook(() => usePlayerProfilePrefs());
      act(() => result.current.saveSubtitleStyle(bigger));
      await act(() => vi.advanceTimersByTimeAsync(600));
      unmount();
      await vi.runAllTimersAsync();
      expect(mutateAsync).toHaveBeenCalledTimes(1);
    });
  });
});
