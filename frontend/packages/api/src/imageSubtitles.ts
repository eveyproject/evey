/**
 * Which subtitle codecs are BITMAPS - pictures, not text.
 *
 * They cannot be delivered as a WebVTT sidecar, so every player has to know the
 * difference before deciding what to ask the server for.
 *
 * They no longer all mean burn-in. A PGS track is decoded server-side once and
 * drawn by the player like any other cue (see isClientDrawableSubtitle), which
 * is what keeps a bitmap subtitle from forcing a video transcode. The rest still
 * need burning in, so they still deny the copy rung.
 *
 * ⚠️ This list MUST match IsImageSubtitle in
 * internal/domain/transcoding/subtitle_codecs.go. It used to be copy-pasted into
 * the web, mobile and TV players, and the copies had drifted: they were missing
 * `dvb_subtitle` and `dvdsub`. `dvb_subtitle` is the canonical name the prober
 * actually stores, so a DVB bitmap track was offered as if it were text, the
 * player asked for a sidecar, the server answered 422, and the subtitle simply
 * never appeared - a track you can select and that does nothing.
 *
 * internal/contractguard/image_subtitle_codecs_test.go fails the build if the
 * two sides diverge again.
 *
 * Both spellings of each format are listed on purpose: ffprobe's canonical
 * codec_name (hdmv_pgs_subtitle, dvd_subtitle, dvb_subtitle) and ffmpeg's
 * encoder aliases (pgssub, dvdsub, dvbsub), because either can reach a client
 * depending on which prober filled the row.
 */
export const IMAGE_SUBTITLE_CODECS: ReadonlySet<string> = new Set([
  "hdmv_pgs_subtitle", "pgssub",
  "dvd_subtitle", "dvdsub",
  "dvb_subtitle", "dvbsub",
  "xsub",
]);

/**
 * isImageSubtitle reports whether a subtitle codec is a bitmap, and therefore
 * needs burn-in rather than a sidecar. Tolerant of case and stray space for the
 * same reason the Go side is: the value comes from a probe, not from us.
 */
export function isImageSubtitle(codec: string | null | undefined): boolean {
  return IMAGE_SUBTITLE_CODECS.has((codec ?? "").trim().toLowerCase());
}

/**
 * Which bitmap codecs a player can DRAW ITSELF, given the decoded-image blob the
 * server produces for them: PGS, VobSub (DVD) and DVB.
 *
 * Not XSUB: nothing decodes it into that blob, and offering it as
 * client-drawable would put a track in the viewer's menu that renders nothing
 * at all - the exact failure the codec list above was written to stop.
 *
 * ⚠️ Must agree with bitmapServable in
 * internal/adapters/driving/http/subtitle_bitmap.go: the client asks, the
 * server answers, and a disagreement is a subtitle that is selected and
 * invisible.
 */
const CLIENT_DRAWABLE_SUBTITLE_CODECS: ReadonlySet<string> = new Set([
  "hdmv_pgs_subtitle", "pgssub", "pgs",
  "dvd_subtitle", "dvdsub",
  "dvb_subtitle", "dvbsub",
]);

/**
 * The server decodes bitmap subtitles out of Matroska only (see
 * serveBitmapSubtitle in internal/adapters/driving/http/subtitle_pgs.go). A
 * bitmap track of an MPEG-TS recording, an AVI or an MP4 has no blob to fetch,
 * so it stays a burn-in. An unknown container (a title probed before the
 * server reported one) keeps the codec's answer.
 */
function bitmapBlobContainer(container: string | null | undefined): boolean {
  const c = (container ?? "").trim().toLowerCase();
  return c === "" || c.includes("matroska") || c.includes("webm");
}

export function isClientDrawableSubtitle(codec: string | null | undefined, container?: string | null): boolean {
  return CLIENT_DRAWABLE_SUBTITLE_CODECS.has((codec ?? "").trim().toLowerCase()) && bitmapBlobContainer(container);
}

/**
 * isBurnInSubtitle reports whether a codec still has to be burnt into the video
 * by the server - a bitmap the player cannot draw. This, and not
 * isImageSubtitle, is what a caller deciding "do I make the server transcode?"
 * should ask now that PGS, VobSub and DVB are drawn client-side.
 */
export function isBurnInSubtitle(codec: string | null | undefined, container?: string | null): boolean {
  return isImageSubtitle(codec) && !isClientDrawableSubtitle(codec, container);
}
