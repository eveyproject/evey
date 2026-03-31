import { useEffect, useRef } from "react";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";

interface UseSubtitleManagerArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  currentSubtitle: number | null;
  externalSubUrl: { url: string; label: string } | null;
  mediaId: string;
  streamToken: string;
  selectedBurnInTrack: number | null;
  transcodeStartTime: number;
  currentTrackIndex: number;
  subtitleStyle: SubtitleStyle;
}

export function useSubtitleManager({
  videoRef,
  currentSubtitle,
  externalSubUrl,
  mediaId,
  streamToken,
  selectedBurnInTrack,
  transcodeStartTime,
  currentTrackIndex,
  subtitleStyle,
}: UseSubtitleManagerArgs) {
  const externalSubUrlRef = useRef<string | null>(null);
  const styleElRef = useRef<HTMLStyleElement | null>(null);

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
    video.appendChild(track);

    // Force browser to load the track (Firefox compat: mode="showing")
    if (track.track) track.track.mode = "showing";

    for (let i = 0; i < video.textTracks.length; i++) {
      if (video.textTracks[i] !== track.track) {
        video.textTracks[i].mode = "hidden";
      }
    }
  }, [currentSubtitle, externalSubUrl, mediaId, streamToken, transcodeStartTime, currentTrackIndex]);

  // Subtitle position (top/bottom) via cue line setting
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const line = subtitleStyle.position === "top" ? 5 : 100;
    for (let i = 0; i < video.textTracks.length; i++) {
      const tt = video.textTracks[i];
      if (tt.mode !== "showing") continue;
      const cues = tt.cues;
      if (!cues) continue;
      for (let j = 0; j < cues.length; j++) {
        const cue = cues[j] as VTTCue;
        if (typeof cue.line !== "undefined") {
          cue.line = line;
          cue.snapToLines = true;
        }
      }
    }
  }, [subtitleStyle.position]);

  // ::cue style injection for subtitle appearance
  useEffect(() => {
    if (!styleElRef.current) {
      styleElRef.current = document.createElement("style");
      document.head.appendChild(styleElRef.current);
    }
    const { fontSize, color, fontFamily, bgColor, bgOpacity, shadow } = subtitleStyle;
    // Only allow alphanumeric, spaces, commas, quotes, hyphens — prevents CSS injection
    const safeFontFamily = fontFamily && /^[a-zA-Z0-9\s,"'-]+$/.test(fontFamily) ? fontFamily : "";
    const fontFamilyRule = safeFontFamily ? `font-family: ${safeFontFamily};` : "";
    const opacity = bgOpacity / 100;
    const r = parseInt(bgColor.slice(1, 3), 16);
    const g = parseInt(bgColor.slice(3, 5), 16);
    const b = parseInt(bgColor.slice(5, 7), 16);
    const bgRule = `background: rgba(${r},${g},${b},${opacity});`;
    const shadowRule = shadow ? "text-shadow: 0 1px 2px rgba(0,0,0,0.8);" : "text-shadow: none;";
    styleElRef.current.textContent = `::cue { font-size: ${fontSize}%; color: ${color}; ${fontFamilyRule} ${bgRule} ${shadowRule} }`;
    return () => {
      styleElRef.current?.remove();
      styleElRef.current = null;
    };
  }, [subtitleStyle]);

  // Cleanup blob URL on unmount
  useEffect(() => {
    return () => {
      if (externalSubUrlRef.current) URL.revokeObjectURL(externalSubUrlRef.current);
    };
  }, []);
}
