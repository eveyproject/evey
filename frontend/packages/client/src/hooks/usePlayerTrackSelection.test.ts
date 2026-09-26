import { describe, it, expect, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { PlayerAudioStream, PlayerSubtitleStream } from "@evey/api";
import { usePlayerTrackSelection } from "./usePlayerTrackSelection";

const audio: PlayerAudioStream[] = [
  { index: 1, trackIndex: 0, language: "eng", isDefault: true },
  { index: 2, trackIndex: 1, language: "fre" },
];
const subs: PlayerSubtitleStream[] = [
  { index: 3, trackIndex: 0, language: "fre", codec: "subrip", burnIn: false },
  { index: 4, trackIndex: 1, language: "eng", codec: "dvd_subtitle", burnIn: true, isDefault: true },
];

type Props = Parameters<typeof usePlayerTrackSelection>[0];
function setup(over: Partial<Props> = {}) {
  const restartWithTracks = vi.fn();
  const initialProps: Props = {
    mediaId: "m1", pick: {}, audioStreams: audio, subtitleStreams: subs,
    audioLangs: [], subtitleLangs: [], restartWithTracks, ...over,
  };
  const hook = renderHook((p: Props) => usePlayerTrackSelection(p), { initialProps });
  return { ...hook, restartWithTracks, initialProps };
}

describe("usePlayerTrackSelection", () => {
  it("follows the profile's languages, then the file's default", () => {
    expect(setup({ audioLangs: ["fr"] }).result.current.selectedAudioIndex).toBe(2);
    const { result } = setup();
    expect(result.current.selectedAudioIndex).toBe(1);
    // The default subtitle is a bitmap: selected, but never sent to the server
    // by itself and never drawn by the client.
    expect(result.current.selectedSubtitleIndex).toBe(4);
    expect(result.current.clientSubtitleIndex).toBeNull();
  });

  it("keeps the title page's pick over languages that land late", () => {
    const { result, rerender, initialProps } = setup({ pick: { audio: 1, subtitle: 3 } });
    rerender({ ...initialProps, audioLangs: ["fr"], subtitleLangs: ["en"] });
    expect(result.current.selectedAudioIndex).toBe(1);
    expect(result.current.selectedSubtitleIndex).toBe(3);
    expect(result.current.clientSubtitleIndex).toBe(0);
  });

  it("burns a picked bitmap subtitle in once, with the selected audio", () => {
    const { rerender, restartWithTracks, initialProps } = setup({ pick: { audio: 2, subtitle: 4 } });
    rerender({ ...initialProps });
    expect(restartWithTracks).toHaveBeenCalledTimes(1);
    expect(restartWithTracks).toHaveBeenCalledWith(2, 4, undefined);
  });

  it("forgets a hand pick on the next title", () => {
    const { result, rerender, initialProps } = setup({ audioLangs: ["fr"] });
    act(() => {
      result.current.userPickedAudioRef.current = true;
      result.current.setSelectedAudioIndex(1);
    });
    rerender({ ...initialProps, audioLangs: ["fr"] });
    expect(result.current.selectedAudioIndex).toBe(1);
    rerender({ ...initialProps, mediaId: "m2", audioStreams: [...audio] });
    expect(result.current.selectedAudioIndex).toBe(2);
    expect(result.current.userPickedAudioRef.current).toBe(false);
  });
});
