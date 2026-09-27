import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";

interface UseSubtitleManagerArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  /** The hls.js instance, when the picture plays through it: a master that
   * declares the packager's windowed WebVTT renditions (X-EVEY-TRACK) serves
   * the selected track segment by segment, instead of the /subtitle route
   * that extracts the whole file first. */
  hlsRef?: React.RefObject<InstanceType<typeof Hls> | null>;
  currentSubtitle: number | null;
  externalSubUrl: { url: string; label: string } | null;
  mediaId: string;
  streamToken: string;
  selectedBurnInTrack: number | null;
  transcodeStartTime: number;
  currentTrackIndex: number;
  subtitleStyle: SubtitleStyle;
  /** Live subtitle resync (ms; positive = subtitles later). Applied to the
   * LOADED cues in place - instant, no track reload, works for embedded and
   * external tracks alike. */
  subtitleOffsetMs?: number;
}

// hlsSubtitleTrackFor finds the hls.js subtitle track the master tagged with
// the picker's ordinal (X-EVEY-TRACK), or -1 when this master has none.
export function hlsSubtitleTrackFor(hls: InstanceType<typeof Hls> | null, ordinal: number | null): number {
  if (!hls || ordinal === null) return -1;
  const wanted = String(ordinal);
  for (const t of hls.subtitleTracks ?? []) {
    const attrs = t.attrs as unknown as Record<string, string | undefined> | undefined;
    if (attrs?.["X-EVEY-TRACK"] === wanted) return t.id;
  }
  return -1;
}

// Cue anchor as a percentage of the video element height (snapToLines=false).
// 88% places the cue top in the upper quarter of the bottom letterbox band on
// scope (2.39:1) content and in the standard subtitle zone on 16:9 - instead
// of the browser default that glues the text to the very bottom edge.
const CUE_LINE_BOTTOM_PCT = 88;
const CUE_LINE_TOP_PCT = 8;

// Native cue text is 5% of the video viewport height per the WebVTT rendering
// rules; fontSize=100 reproduces that baseline.
const NATIVE_CUE_HEIGHT_RATIO = 0.05;
const MIN_CUE_FONT_PX = 13;

// cueFontSizePx converts the user's percentage preference into an absolute
// pixel size for ::cue. An absolute size is required: percentage font-size in
// ::cue resolves against the page default (16px) in Chromium - not against
// the native 5%-of-video cue size - which rendered subtitles tiny on any
// large player. Exported for tests.
export function cueFontSizePx(videoHeightPx: number, fontSizePct: number): number {
  if (videoHeightPx <= 0) return 0;
  return Math.max(MIN_CUE_FONT_PX, videoHeightPx * NATIVE_CUE_HEIGHT_RATIO * (fontSizePct / 100));
}

// CueBaseTimes remembers each cue's SOURCE timing the first time an offset
// touches it, so every application computes absolute source+offset instead of
// chaining deltas - a head cue clamped at zero would otherwise lose the
// clamped amount and drift on the way back.
export type CueBaseTimes = WeakMap<TextTrackCue, { start: number; end: number }>;

// applySubtitleOffset sets every cue of the loaded tracks to its SOURCE time
// plus totalOffsetSec. Cue times are mutated IN PLACE (the browser
// re-schedules them live), so a stepper change lands on the very next cue
// without reloading the track. Times clamp at 0 for display, but the stored
// base keeps the operation exactly reversible. Exported for tests.
export function applySubtitleOffset(
  video: HTMLVideoElement,
  totalOffsetSec: number,
  base: CueBaseTimes,
) {
  for (let i = 0; i < video.textTracks.length; i++) {
    const cues = video.textTracks[i].cues;
    if (!cues) continue;
    for (let j = 0; j < cues.length; j++) {
      const cue = cues[j];
      let src = base.get(cue);
      if (!src) {
        src = { start: cue.startTime, end: cue.endTime };
        base.set(cue, src);
      }
      cue.startTime = Math.max(0, src.start + totalOffsetSec);
      cue.endTime = Math.max(cue.startTime, src.end + totalOffsetSec);
    }
  }
}

// applyCuePosition re-anchors every cue of the showing tracks. Percentage
// lines (snapToLines=false) are used instead of line numbers: the previous
// `line=100, snapToLines=true` meant "the 100th text line from the top",
// which the browser clamps to the bottom edge. Exported for tests.
// A cue the file itself places in the upper half (a WebVTT `line:N%` under
// 50, which is how the server renders an ASS {\an8} sign or caption) keeps
// that placement under the bottom preference: the author put it there so it
// would not collide with the dialogue. Remembered on first sight, because the
// pass below rewrites every cue's line.
const authoredUpper = new WeakMap<VTTCue, boolean>();

export function applyCuePosition(video: HTMLVideoElement, position: SubtitleStyle["position"]) {
  const line = position === "top" ? CUE_LINE_TOP_PCT : CUE_LINE_BOTTOM_PCT;
  for (let i = 0; i < video.textTracks.length; i++) {
    const tt = video.textTracks[i];
    if (tt.mode !== "showing") continue;
    const cues = tt.cues;
    if (!cues) continue;
    for (let j = 0; j < cues.length; j++) {
      const cue = cues[j] as VTTCue;
      if (typeof cue.line === "undefined") continue;
      let upper = authoredUpper.get(cue);
      if (upper === undefined) {
        upper = cue.snapToLines === false && typeof cue.line === "number" && cue.line < 50;
        authoredUpper.set(cue, upper);
      }
      if (upper && position === "bottom") continue;
      cue.snapToLines = false; // before line: percentages outside 0-100 throw
      cue.line = line;
    }
  }
}

export function useSubtitleManager({
  videoRef,
  hlsRef,
  currentSubtitle,
  externalSubUrl,
  mediaId,
  streamToken,
  selectedBurnInTrack,
  transcodeStartTime,
  currentTrackIndex,
  subtitleStyle,
  subtitleOffsetMs = 0,
}: UseSubtitleManagerArgs) {
  const externalSubUrlRef = useRef<string | null>(null);
  // Source timings of the cues currently loaded - lets every offset change
  // apply the ABSOLUTE value (exactly reversible across the zero clamp). A
  // fresh track load brings fresh cue objects, so stale entries just fall out
  // of the WeakMap.
  const cueBaseTimesRef = useRef<CueBaseTimes>(new WeakMap());
  const styleElRef = useRef<HTMLStyleElement | null>(null);
  // The track effect must see the current position without re-creating the
  // <track> element on a position toggle.
  const positionRef = useRef(subtitleStyle.position);
  positionRef.current = subtitleStyle.position;
  const offsetRef = useRef(subtitleOffsetMs);
  offsetRef.current = subtitleOffsetMs;

  // Live resync: re-anchor every loaded cue to source time + offset.
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    applySubtitleOffset(video, subtitleOffsetMs / 1000, cueBaseTimesRef.current);
  }, [subtitleOffsetMs, videoRef]);

  // The master's subtitle renditions arrive with the manifest, after this
  // hook first ran: re-run the track effect when hls.js learns them, and keep
  // the cues styled as their segments land.
  const [hlsSubsVersion, setHlsSubsVersion] = useState(0);
  // Until the manifest is parsed the master's renditions are unknown: the
  // /subtitle fallback must not fire meanwhile, it would start a whole-file
  // extraction on the server for a track the packager serves seconds later.
  const hlsSubsKnownRef = useRef(false);
  useEffect(() => {
    const hls = hlsRef?.current;
    if (!hls) return;
    hlsSubsKnownRef.current = false;
    const bump = () => {
      hlsSubsKnownRef.current = true;
      setHlsSubsVersion((v) => v + 1);
    };
    const onCues = () => {
      const video = videoRef.current;
      if (!video) return;
      applyCuePosition(video, positionRef.current);
      if (offsetRef.current !== 0) applySubtitleOffset(video, offsetRef.current / 1000, cueBaseTimesRef.current);
    };
    hls.on(Hls.Events.MANIFEST_PARSED, bump);
    hls.on(Hls.Events.SUBTITLE_TRACKS_UPDATED, bump);
    hls.on(Hls.Events.CUES_PARSED, onCues);
    return () => {
      hls.off(Hls.Events.MANIFEST_PARSED, bump);
      hls.off(Hls.Events.SUBTITLE_TRACKS_UPDATED, bump);
      hls.off(Hls.Events.CUES_PARSED, onCues);
    };
    // The instance is recreated with the stream; the token change tracks that.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hlsRef, streamToken, currentTrackIndex, transcodeStartTime, videoRef]);

  // Dynamic <track> element management
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    const existing = video.querySelector("track[data-dynamic]");
    if (existing) existing.remove();

    if (externalSubUrlRef.current && externalSubUrlRef.current !== externalSubUrl?.url) {
      URL.revokeObjectURL(externalSubUrlRef.current);
      externalSubUrlRef.current = null;
    }

    const isBurnIn = selectedBurnInTrack !== null;

    // Packager rendition first: the cues come with the segments, no full read
    // of the film behind the subtitle button.
    const hls = hlsRef?.current ?? null;
    const hlsTrack = externalSubUrl || isBurnIn ? -1 : hlsSubtitleTrackFor(hls, currentSubtitle);
    if (hls) {
      if (hls.subtitleTrack !== hlsTrack) hls.subtitleTrack = hlsTrack;
      hls.subtitleDisplay = hlsTrack >= 0;
    }
    if (hlsTrack >= 0) {
      cueBaseTimesRef.current = new WeakMap();
      applyCuePosition(video, positionRef.current);
      return;
    }
    if (hls && !hlsSubsKnownRef.current && !externalSubUrl && !isBurnIn) return;

    const src = externalSubUrl
      ? externalSubUrl.url
      : currentSubtitle !== null && !isBurnIn && streamToken
        ? `/api/v1/media/${mediaId}/subtitle/${currentSubtitle}?token=${streamToken}`
        : null;

    if (!src) return;
    if (externalSubUrl) externalSubUrlRef.current = externalSubUrl.url;

    const track = document.createElement("track");
    track.setAttribute("data-dynamic", "1");
    track.kind = "subtitles";
    track.label = externalSubUrl ? externalSubUrl.label : "Selected";
    track.src = src;
    track.default = true;
    // Re-anchor the cues once the VTT is parsed - without this, a freshly
    // loaded track keeps the browser default (glued to the bottom edge) until
    // the user happens to toggle the position setting.
    cueBaseTimesRef.current = new WeakMap();
    track.addEventListener("load", () => {
      applyCuePosition(video, positionRef.current);
      // A freshly parsed track carries the source timings: bring it to the
      // user's current resync setting.
      if (offsetRef.current !== 0) {
        applySubtitleOffset(video, offsetRef.current / 1000, cueBaseTimesRef.current);
      }
    });
    video.appendChild(track);

    // Force browser to load the track (Firefox compat: mode="showing")
    if (track.track) track.track.mode = "showing";

    for (let i = 0; i < video.textTracks.length; i++) {
      if (video.textTracks[i] !== track.track) {
        video.textTracks[i].mode = "hidden";
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentSubtitle, externalSubUrl, mediaId, streamToken, transcodeStartTime, currentTrackIndex, hlsSubsVersion]);

  // Subtitle position (top/bottom) live toggle
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    applyCuePosition(video, subtitleStyle.position);
  }, [subtitleStyle.position, videoRef]);

  // ::cue style injection for subtitle appearance. Font size is computed in
  // px from the live element height (see cueFontSizePx) and tracks resizes /
  // fullscreen transitions via ResizeObserver.
  useEffect(() => {
    if (!styleElRef.current) {
      styleElRef.current = document.createElement("style");
      document.head.appendChild(styleElRef.current);
    }
    const { fontSize, color, fontFamily, bgColor, bgOpacity, shadow } = subtitleStyle;
    // Only allow alphanumeric, spaces, commas, quotes, hyphens - prevents CSS injection
    const safeFontFamily = fontFamily && /^[a-zA-Z0-9\s,"'-]+$/.test(fontFamily) ? fontFamily : "";
    const fontFamilyRule = safeFontFamily ? `font-family: ${safeFontFamily};` : "";
    const opacity = bgOpacity / 100;
    const r = parseInt(bgColor.slice(1, 3), 16);
    const g = parseInt(bgColor.slice(3, 5), 16);
    const b = parseInt(bgColor.slice(5, 7), 16);
    const bgRule = `background: rgba(${r},${g},${b},${opacity});`;
    const shadowRule = shadow ? "text-shadow: 0 1px 2px rgba(0,0,0,0.8);" : "text-shadow: none;";

    const apply = () => {
      const px = cueFontSizePx(videoRef.current?.clientHeight ?? 0, fontSize);
      // Element not measurable yet → fall back to the percentage (better than
      // nothing); the ResizeObserver re-applies as soon as it has a size.
      const fontRule = px > 0 ? `font-size: ${px.toFixed(1)}px;` : `font-size: ${fontSize}%;`;
      if (styleElRef.current) {
        styleElRef.current.textContent = `::cue { ${fontRule} color: ${color}; ${fontFamilyRule} ${bgRule} ${shadowRule} }`;
      }
    };
    apply();

    let ro: ResizeObserver | undefined;
    if (videoRef.current && typeof ResizeObserver !== "undefined") {
      ro = new ResizeObserver(apply);
      ro.observe(videoRef.current);
    }
    return () => {
      ro?.disconnect();
      styleElRef.current?.remove();
      styleElRef.current = null;
    };
  }, [subtitleStyle, videoRef]);

  // Cleanup blob URL on unmount
  useEffect(() => {
    return () => {
      if (externalSubUrlRef.current) URL.revokeObjectURL(externalSubUrlRef.current);
    };
  }, []);
}
