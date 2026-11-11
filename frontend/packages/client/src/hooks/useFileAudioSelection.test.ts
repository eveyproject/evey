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
    expect(nativeTrackToApply(tracks, tracks[0], streams[1], streams)?.label).toBe("ENG");
  });

  it("leaves the player alone when it already plays the picked stream", () => {
    expect(nativeTrackToApply(tracks, tracks[1], streams[1], streams)).toBeNull();
  });

  it("waits for the native tracks", () => {
    expect(nativeTrackToApply([], null, streams[1], streams)).toBeNull();
  });
});

// The player lists its tracks renderer by renderer, not in file order: with the
// ac3go renderer ahead of MediaCodec, an E-AC-3 track comes first even when
// it is the file's second audio track.
describe("nativeTrackToApply on a player that reorders the file's tracks", () => {
  const file: PlayerAudioStream[] = [
    { index: 1, trackIndex: 0, language: "en", title: "VO", codec: "aac" },
    { index: 2, trackIndex: 1, language: "fr-FR", title: "VF", codec: "eac3", isDefault: true },
  ];
  const reordered: NativeAudioTrack[] = [
    { id: "2", language: "fr", label: "French", name: "VF" },
    { id: "1", language: "en", label: "English", name: "VO" },
  ];

  it("plays the French track when the French track is picked", () => {
    expect(nativeTrackToApply(reordered, null, file[1], file)?.id).toBe("2");
  });

  it("plays the original track when the original track is picked", () => {
    expect(nativeTrackToApply(reordered, null, file[0], file)?.id).toBe("1");
  });

  it("tells two tracks of one language apart by their title", () => {
    const twoFrench: PlayerAudioStream[] = [
      { index: 1, trackIndex: 0, language: "fr", title: "VFQ", codec: "aac" },
      { index: 2, trackIndex: 1, language: "fr", title: "VFF", codec: "eac3" },
    ];
    const native: NativeAudioTrack[] = [
      { id: "2", language: "fr", name: "VFF" },
      { id: "1", language: "fr", name: "VFQ" },
    ];
    expect(nativeTrackToApply(native, null, twoFrench[0], twoFrench)?.id).toBe("1");
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

// The film a phone crashed on: an English DTS first, the French AC-3 flagged
// default second. The player lists the AC-3 first (its ac3go renderer comes
// ahead of MediaCodec), so the French stream's file position names the DTS.
describe("useFileAudioSelection on a file with a track the device cannot decode", () => {
  const film: PlayerAudioStream[] = [
    { index: 1, trackIndex: 0, language: "eng", title: "DTS 1509 Kbps", codec: "dts" },
    { index: 4, trackIndex: 1, language: "fre", title: "Ac3 640 Kbps Truefrench", codec: "ac3", isDefault: true },
  ];
  const native: NativeAudioTrack[] = [
    { id: "5", language: "fr", name: "Ac3 640 Kbps Truefrench" },
    { id: "2", language: "en", name: "DTS 1509 Kbps" },
  ];
  const phone = ["aac", "mp3", "opus", "flac", "vorbis", "ac3", "eac3"];

  it("puts the French AC-3 on the player, never the DTS at its file position", () => {
    const player = fakePlayer();
    renderHook(() => useFileAudioSelection(player, { enabled: true, audioStreams: film, selectedAudioIndex: 4, declaredAudioCodecs: phone }));
    act(() => player.publish(native));
    expect(player.audioTrack?.id).toBe("5");
  });

  it("never forces the DTS track, even when it is the one selected", () => {
    const player = fakePlayer();
    renderHook(() => useFileAudioSelection(player, { enabled: true, audioStreams: film, selectedAudioIndex: 1, declaredAudioCodecs: phone }));
    act(() => player.publish(native));
    // The player stays on the track it opened by itself, one it decodes.
    expect(player.audioTrack?.id).toBe("5");
  });

  it("forces the DTS track on a device that declares it", () => {
    const player = fakePlayer();
    renderHook(() => useFileAudioSelection(player, { enabled: true, audioStreams: film, selectedAudioIndex: 1, declaredAudioCodecs: ["dts", ...phone] }));
    act(() => player.publish(native));
    expect(player.audioTrack?.id).toBe("2");
  });
});

// The player reads a language from the old Matroska element only and calls
// every other track "en": the file's own track number still names the track.
describe("nativeTrackToApply by the file's track number", () => {
  const film: PlayerAudioStream[] = [
    { index: 1, trackIndex: 0, language: "eng", codec: "eac3", sourceTrackId: 2 },
    { index: 4, trackIndex: 1, language: "fre", codec: "ac3", sourceTrackId: 5, isDefault: true },
  ];
  const allEnglish: NativeAudioTrack[] = [
    { id: "5", language: "en" },
    { id: "2", language: "en" },
  ];

  it("plays the French track the player believes is English", () => {
    expect(nativeTrackToApply(allEnglish, null, film[1], film)?.id).toBe("5");
    expect(nativeTrackToApply(allEnglish, null, film[0], film)?.id).toBe("2");
  });

  it("falls back to the language when the server has no track number", () => {
    const noIds = film.map((s) => ({ ...s, sourceTrackId: undefined }));
    expect(nativeTrackToApply([{ id: "a", language: "fr" }, { id: "b", language: "en" }], null, noIds[1], noIds)?.id).toBe("a");
  });
});
