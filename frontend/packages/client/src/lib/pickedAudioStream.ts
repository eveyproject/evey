import { defaultAudioCandidates, type V1StreamInfo } from "@evey/api";

// The audio track the master will serve: the picked one, else the
// default-flagged one, else the first, a commentary track only when every
// track is one. The server resolves the same default (DefaultAudioTrack)
// before it decides, so every player reads a verdict against the track it
// was asked for.
export function pickedAudioStream(info: V1StreamInfo, audioStreamIndex: number | undefined) {
  const audio = (info.mediaStreams ?? []).filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
  return audioStreamIndex != null && audioStreamIndex > 0
    ? audio.find((s) => s.index === audioStreamIndex)
    : (() => {
        const candidates = defaultAudioCandidates(audio);
        return candidates.find((s) => s.isDefault) ?? candidates[0];
      })();
}
