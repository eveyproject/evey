import { describe, it, expect } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { PlayerAudioStream } from "@evey/api";
import { nativeTrackToApply, useFileAudioSelection, type AudioTrackPlayer, type NativeAudioTrack } from "./useFileAudioSelection";

const tracks: NativeAudioTrack[] = [
  { id: "0", language: "fre", label: "VFF" },
  { id: "1", language: "eng", label: "ENG" },
];
const streams: PlayerAudioStream[] = [
  { index: 1, trackIndex: 0, language: "fre", isDefault: true },
  { index: 2, trackIndex: 1, language: "eng" },
];

// The file opens on its default track, and announces its tracks late.
function fakePlayer(initial: NativeAudioTrack[] = []) {
  let listener: (() => void) | null = null;
  const player: AudioTrackPlayer & { publish: (t: NativeAudioTrack[]) => void } = {
    audioTrack: null,
    availableAudioTracks: initial,
    addListener: (_event, cb) => {
      listener = cb;
      return { remove: () => { listener = null; } };
    },
    publish(t) {
      player.availableAudioTracks = t;
      player.audioTrack = t[0] ?? null;
      listener?.();
    },
  };
  return player;
}

describe("nativeTrackToApply", () => {
  it("switches to the picked stream when the player opened on another", () => {
    expect(nativeTrackToApply(tracks, tracks[0], 1, "eng")?.label).toBe("ENG");
  });

  it("leaves the player alone when it already plays the picked stream", () => {
    expect(nativeTrackToApply(tracks, tracks[1], 1, "eng")).toBeNull();
  });

  it("waits for the native tracks", () => {
    expect(nativeTrackToApply([], null, 1, "eng")).toBeNull();
  });
});

describe("useFileAudioSelection", () => {
  it("puts the title page's pick on a file that opened on its default track", () => {
    const player = fakePlayer();
    renderHook(() => useFileAudioSelection(player, { enabled: true, audioStreams: streams, selectedAudioIndex: 2 }));
    expect(player.audioTrack).toBeNull();
    act(() => player.publish(tracks));
    expect(player.audioTrack?.label).toBe("ENG");
  });

  it("follows a change made in the player", () => {
    const player = fakePlayer();
    const hook = renderHook((sel: number) => useFileAudioSelection(player, { enabled: true, audioStreams: streams, selectedAudioIndex: sel }), { initialProps: 1 });
    act(() => player.publish(tracks));
    expect(player.audioTrack?.label).toBe("VFF");
    hook.rerender(2);
    expect(player.audioTrack?.label).toBe("ENG");
  });

  // An HLS URL already names the track: the manifest's own order is not the file's.
  it("does not touch a stream that is not the file", () => {
    const player = fakePlayer();
    renderHook(() => useFileAudioSelection(player, { enabled: false, audioStreams: streams, selectedAudioIndex: 2 }));
    act(() => player.publish(tracks));
    expect(player.audioTrack?.label).toBe("VFF");
  });
});
