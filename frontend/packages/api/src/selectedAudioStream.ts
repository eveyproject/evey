// Which audio stream the stream-info panel should describe.
//
// All three panels used to do `mediaStreams.find(type === AUDIO)` - the file's
// FIRST audio track, whatever the viewer was actually listening to. Prod,
// 2026-09-09: a release whose first track is a TrueHD Atmos 7.1 (eng) reported
// "TRUEHD / 7.1 / eng" while the viewer was on the French AC-3, and contradicted
// the "decoded client" badge next to it, which IS computed from the real
// selection. The panel exists to say what is playing; it said the opposite.
//
// Two index conventions are in play and mixing them is silent:
//
//   absolute - the media-stream index (video 0, audio 1, subtitle 5...). What
//              mobile and TV carry in their selection state.
//   relative - the AUDIO-relative ordinal (first audio track 0). What the web
//              player's hls.js track selection carries.
//
// Pass whichever you hold; passing neither (or one that matches nothing) falls
// back to the first audio track, which is the old behaviour - now a fallback
// rather than the rule.

export interface AudioStreamLike {
  type?: string;
  index?: number;
}

export function pickAudioStream<T extends AudioStreamLike>(
  streams: T[] | undefined,
  sel: { absolute?: number | null; relative?: number | null },
): T | undefined {
  const audio = (streams ?? []).filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
  if (audio.length === 0) return undefined;

  if (sel.absolute != null) {
    const hit = audio.find((s) => s.index === sel.absolute);
    if (hit) return hit;
  }
  if (sel.relative != null && sel.relative >= 0 && sel.relative < audio.length) {
    return audio[sel.relative];
  }
  return audio[0];
}
