// The server's shed contract (HTTP 503 on a refused playback start, the code in
// the X-Evey-Error header and the JSON body - writeShed, transcode_vod_admission.go):
// - too_many_streams: the box can't prepare one more stream because of OTHER
//   concurrent playbacks.
// - file_not_ready: this file's seek index is still being built.
// - calibrating: the server is benchmarking its encode capacity (~2 min).
// - file_needs_remux: the file is stored in one-track blocks; an admin re-muxes
//   it from the media-health page.
// - file_missing: the title's file is gone from disk (GetStreamInfo's
//   MEDIA_FILE_MISSING refusal, not a 503): nothing to retry.
// - media_unavailable: GetStreamInfo answered a plain 404, the title is no
//   longer in the catalogue (pruned while a list still showed it). Plain on
//   purpose: a parental block answers the same 404, so the copy stays neutral.
// Never surface the server's `message` (English, technical): the player picks
// its copy from the code.
export const SHED_CODES = ["too_many_streams", "file_not_ready", "calibrating", "file_needs_remux", "file_missing", "media_unavailable"] as const;
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
  file_missing: "fileMissing",
  media_unavailable: "mediaUnavailable",
};

// goneReasonFromError reads a refused playback start the shared fetcher threw:
// MEDIA_FILE_MISSING is a deleted file, any other 404 a title out of the
// catalogue (or not this profile's to play: the same 404 on purpose).
export function goneReasonFromError(err: unknown): ShedCode | null {
  const e = err as { httpStatus?: number; errorCode?: string } | null | undefined;
  if (e?.errorCode === "media_file_missing") return "file_missing";
  if (e?.httpStatus === 404) return "media_unavailable";
  return null;
}

// isTitleGone: no retry brings the title back. The player offers the next
// episode or the way back, and refreshes the lists that still show it.
export function isTitleGone(reason: ShedReason | undefined): boolean {
  return reason === "file_missing" || reason === "media_unavailable";
}

export function shedI18nKey(reason: ShedCode, part: "title" | "body"): string {
  return `player.shed.${SHED_I18N_GROUP[reason]}.${part}`;
}
