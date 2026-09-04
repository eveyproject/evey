/**
 * Which subtitle codecs are BITMAPS - pictures, not text.
 *
 * They cannot be delivered as a WebVTT sidecar and cannot be drawn by a client:
 * the server has to burn them onto the video, which forces a video transcode.
 * Every player therefore has to know the difference before it decides whether to
 * ask for a sidecar or to request burn-in.
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
