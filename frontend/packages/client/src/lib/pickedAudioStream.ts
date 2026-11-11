import { defaultAudioCandidates, type V1StreamInfo } from "@evey/api";

// The audio track the master will serve: the picked one, else the
// default-flagged one, else the first, a commentary track only when every
// track is one. The server resolves the same default (DefaultAudioTrack)
// before it decides, so every player reads a verdict against the track it
// was asked for.
// A server that names the track it judged (audioStreamIndex on the verdict)
// is read first: it also weighs the profile's languages, which a player that
// only reads the file's flags would miss.
export function pickedAudioStream(info: V1StreamInfo, audioStreamIndex: number | undefined) {
  const audio = (info.mediaStreams ?? []).filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
  const picked = audioStreamIndex != null && audioStreamIndex >= 0 ? audio.find((s) => s.index === audioStreamIndex) : undefined;
  if (picked) return picked;
  const judged = info.audioStreamIndex != null ? audio.find((s) => s.index === info.audioStreamIndex) : undefined;
  if (judged) return judged;
  const candidates = defaultAudioCandidates(audio);
  return candidates.find((s) => s.isDefault) ?? candidates[0];
}
