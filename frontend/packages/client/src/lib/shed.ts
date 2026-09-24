// The server's shed contract (HTTP 503 on a refused playback start, the code in
// the X-Evey-Error header and the JSON body - writeShed, transcode_vod_admission.go):
// - too_many_streams: the box can't prepare one more stream because of OTHER
//   concurrent playbacks.
// - file_not_ready: this file's seek index is still being built.
// - calibrating: the server is benchmarking its encode capacity (~2 min).
// - file_needs_remux: the file is stored in one-track blocks; an admin re-muxes
//   it from the media-health page.
// Never surface the server's `message` (English, technical): the player picks
// its copy from the code.
export const SHED_CODES = ["too_many_streams", "file_not_ready", "calibrating", "file_needs_remux"] as const;
export type ShedCode = (typeof SHED_CODES)[number];
export type ShedReason = ShedCode | null;

export function isShedCode(code: string | null | undefined): code is ShedCode {
  return (SHED_CODES as readonly string[]).includes(code ?? "");
}

const SHED_I18N_GROUP: Record<ShedCode, string> = {
  too_many_streams: "tooManyStreams",
  file_not_ready: "fileNotReady",
  calibrating: "calibrating",
  file_needs_remux: "fileNeedsRemux",
};

export function shedI18nKey(reason: ShedCode, part: "title" | "body"): string {
  return `player.shed.${SHED_I18N_GROUP[reason]}.${part}`;
}
