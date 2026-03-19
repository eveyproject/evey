import { useCallback, useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import { useAuthStore, toast } from "@evey/store";
import type { V1StreamInfo } from "@evey/api/src/generated/playback.schemas";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { PlayerControls } from "./PlayerControls";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerControls";
import { ProgressReporter } from "./ProgressReporter";
import { NextEpisodeOverlay } from "./NextEpisodeOverlay";
import { SkipIntroButton } from "./SkipIntroButton";
import { KeyboardShortcutsOverlay } from "./KeyboardShortcutsOverlay";
import type { Chapter } from "@/hooks/useChapters";
import { usePlaybackSettings } from "@/hooks/usePlaybackSettings";
import { canPlayHevc } from "@/utils/codec";
import {
  getPreferredAudioLang,
  setPreferredAudioLang,
  getPreferredSubtitleLang,
  setPreferredSubtitleLang,
  getSeriesTrackPrefs,
  setSeriesTrackPrefs,
  getSubtitleStyle,
  setSubtitleStyle,
  type SubtitleStyle,
} from "@/hooks/usePlaybackPrefs";

const QUALITY_LEVELS = [0, 1080, 720, 480, 360];
const MAX_FALLBACK_ATTEMPTS = 3;

interface AudioTrack {
  id: string;
  enabled: boolean;
}

interface AudioTrackList {
  length: number;
  [index: number]: AudioTrack;
}

interface HTMLVideoElementWithAudioTracks extends HTMLVideoElement {
  audioTracks?: AudioTrackList;
}

const SPEED_OPTIONS = [0.5, 0.75, 1, 1.25, 1.5, 2];

export type { AudioStream, SubtitleStream, EpisodeNavItem, SeasonNavItem };

interface HLSPlayerProps {
  streamInfo: V1StreamInfo;
  sessionId: string;
  mediaId: string;
  startPositionMs?: number;
  knownDurationMs?: number;
  filePath?: string;
  onEnded?: () => void;
  audioStreams?: AudioStream[];
  subtitleStreams?: SubtitleStream[];
  title?: string;
  onClose?: () => void;
  seasons?: SeasonNavItem[];
  currentSeasonId?: string;
  episodes?: EpisodeNavItem[];
  currentEpisodeId?: string;
  onSeasonChange?: (id: string) => void;
  onEpisodeChange?: (id: string) => void;
  chapters?: Chapter[];
  seriesId?: string;
  onVideoRef?: (el: HTMLVideoElement | null) => void;
}

export function HLSPlayer({
  streamInfo,
  sessionId,
  mediaId,
  startPositionMs = 0,
  knownDurationMs,
  filePath,
  onEnded,
  audioStreams = [],
  subtitleStreams = [],
  title,
  onClose,
  seasons,
  currentSeasonId,
  episodes,
  currentEpisodeId,
  onSeasonChange,
  onEpisodeChange,
  chapters = [],
  seriesId,
  onVideoRef,
}: HLSPlayerProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const hlsRef = useRef<InstanceType<typeof Hls> | null>(null);
  // Track current playback position across reloads (cleanup resets video.currentTime to 0)
  const positionRef = useRef(startPositionMs / 1000);
  const knownDurationSecRef = useRef(knownDurationMs ? knownDurationMs / 1000 : 0);
  knownDurationSecRef.current = knownDurationMs ? knownDurationMs / 1000 : 0;
  const [transcodeMode, setTranscodeMode] = useState(
    streamInfo.playMethod !== "PLAY_METHOD_DIRECT_PLAY" &&
    streamInfo.playMethod !== "PLAY_METHOD_DIRECT_STREAM"
  );
  const [overlayTime, setOverlayTime] = useState(0);
  const [overlayDuration, setOverlayDuration] = useState(0);
  const defaultAudio = (() => {
    const seriesPrefs = seriesId ? getSeriesTrackPrefs(seriesId) : null;
    const prefLang = seriesPrefs?.audioLang ?? getPreferredAudioLang();
    if (prefLang) {
      const match = audioStreams.find((s) => (s.language ?? s.title ?? "").toLowerCase() === prefLang.toLowerCase());
      if (match) return match;
    }
    return audioStreams.find((s) => s.isDefault) ?? audioStreams[0];
  })();
  const [currentAudioIndex, setCurrentAudioIndex] = useState(defaultAudio?.index ?? 0);
  const [currentTrackIndex, setCurrentTrackIndex] = useState(defaultAudio?.trackIndex ?? 0);
  const [currentQuality, setCurrentQuality] = useState(0); // 0 = original
  const [autoQuality, setAutoQuality] = useState(true);
  const [currentSubtitle, setCurrentSubtitle] = useState<number | null>(() => {
    const seriesPrefs = seriesId ? getSeriesTrackPrefs(seriesId) : null;
    if (seriesPrefs && "subtitleLang" in seriesPrefs) {
      if (seriesPrefs.subtitleLang === null || seriesPrefs.subtitleLang === undefined) return null;
      const match = subtitleStreams.find((s) => (s.language ?? s.title ?? "").toLowerCase() === seriesPrefs.subtitleLang!.toLowerCase());
      if (match) return match.trackIndex;
    }
    const prefLang = getPreferredSubtitleLang();
    if (!prefLang) return null;
    const match = subtitleStreams.find((s) => (s.language ?? s.title ?? "").toLowerCase() === prefLang.toLowerCase());
    return match?.trackIndex ?? null;
  });
  // Burn-in subtitle: the selected subtitle track index when it's an image-based subtitle (PGS/VobSub)
  const selectedBurnInTrack = (() => {
    if (currentSubtitle === null) return null;
    const sub = subtitleStreams.find((s) => s.trackIndex === currentSubtitle);
    return sub?.burnIn ? currentSubtitle : null;
  })();

  const [playbackSpeed, setPlaybackSpeed] = useState(1);
  const anyPanelOpenRef = useRef(false);
  const closePanelRef = useRef<(() => void) | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const [externalSubUrl, setExternalSubUrl] = useState<{ url: string; label: string } | null>(null);
  const externalSubUrlRef = useRef<string | null>(null);
  const [subtitleStyle, setSubtitleStyleState] = useState<SubtitleStyle>(() => getSubtitleStyle());
  const styleElRef = useRef<HTMLStyleElement | null>(null);

  function updateSubtitleStyle(patch: Partial<SubtitleStyle>) {
    setSubtitleStyleState((prev) => {
      const next = { ...prev, ...patch };
      setSubtitleStyle(next);
      return next;
    });
  }
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [isBuffering, setIsBuffering] = useState(false);
  const [fatalError, setFatalError] = useState(false);
  const fallbackCountRef = useRef(0);
  const lastFallbackTimeRef = useRef(0);
  const wasPausedRef = useRef(false);
  const playbackSpeedRef = useRef(playbackSpeed);
  playbackSpeedRef.current = playbackSpeed;
  const subtitleStreamsRef = useRef(subtitleStreams);
  subtitleStreamsRef.current = subtitleStreams;
  const currentSubtitleRef = useRef(currentSubtitle);
  currentSubtitleRef.current = currentSubtitle;
  const accessToken = useAuthStore((s) => s.accessToken);
  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 10;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 10;
  const skipFwdRef = useRef(skipFwd);
  skipFwdRef.current = skipFwd;
  const skipBwdRef = useRef(skipBwd);
  skipBwdRef.current = skipBwd;
  // Offset applied when we restart transcode from a non-zero position
  const transcodeOffsetRef = useRef(0);
  const currentQualityRef = useRef(currentQuality);
  currentQualityRef.current = currentQuality;
  // initialized below; ref object is stable, .current is updated after function def
  const attemptQualityFallbackRef = useRef<(q: number) => boolean>(() => false);

  const isDirectPlay = streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY";
  const isDirectStream = streamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM";

  // Direct play / direct stream path
  useEffect(() => {
    const video = videoRef.current;
    if (!video || transcodeMode) return;

    let hlsInstance: InstanceType<typeof Hls> | null = null;

    // HLS only when neither direct play nor direct stream — those use native video.src
    if (!isDirectPlay && !isDirectStream && streamInfo.transcodeUrl && Hls.isSupported()) {
      hlsInstance = new Hls();
      hlsRef.current = hlsInstance;
      hlsInstance.loadSource(streamInfo.transcodeUrl);
      hlsInstance.attachMedia(video);
      hlsInstance.on(Hls.Events.MANIFEST_PARSED, () => {
        video.currentTime = startPositionMs / 1000;
        video.play().catch(() => {});
      });
      hlsInstance.on(Hls.Events.AUDIO_TRACKS_UPDATED, (_, data) => {
        if (data.audioTracks.length === 0) setTranscodeMode(true);
      });
      hlsInstance.on(Hls.Events.ERROR, (_, data) => {
        if (!data.fatal) return;
        console.warn("[player] HLS fatal error:", data.type, data.details);
        setTranscodeMode(true);
      });
    } else if (streamInfo.directPlayUrl) {
      video.src = streamInfo.directPlayUrl;
      video.currentTime = startPositionMs / 1000;
      video.play().catch(() => {});
    }

    let canPlayTimer: ReturnType<typeof setTimeout> | null = null;
    function handleCanPlay() {
      canPlayTimer = setTimeout(() => {
        const v = videoRef.current as HTMLVideoElementWithAudioTracks | null;
        if (!v || transcodeMode) return;
        const tracks = v.audioTracks;
        if (tracks && tracks.length === 0) setTranscodeMode(true);
      }, 3000);
    }
    function handleEnded() { onEnded?.(); }
    function handleError() {
      const e = video!.error;
      if (e) {
        console.warn("[player] direct play error:", e.code, e.message);
        setTranscodeMode(true);
      }
    }
    function handleTimeUpdate() { positionRef.current = video!.currentTime; }
    function handleWaiting() { setIsBuffering(true); }
    function handlePlaying() { setIsBuffering(false); }
    video.addEventListener("timeupdate", handleTimeUpdate);
    video.addEventListener("ended", handleEnded);
    video.addEventListener("canplay", handleCanPlay);
    video.addEventListener("error", handleError);
    video.addEventListener("waiting", handleWaiting);
    video.addEventListener("playing", handlePlaying);
    video.addEventListener("canplaythrough", handlePlaying);

    return () => {
      if (canPlayTimer !== null) clearTimeout(canPlayTimer);
      video.removeEventListener("timeupdate", handleTimeUpdate);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("canplay", handleCanPlay);
      video.removeEventListener("error", handleError);
      video.removeEventListener("waiting", handleWaiting);
      video.removeEventListener("playing", handlePlaying);
      video.removeEventListener("canplaythrough", handlePlaying);
      if (hlsInstance) {
        hlsInstance.destroy();
        hlsRef.current = null;
      }
    };
  }, [streamInfo, startPositionMs, onEnded, transcodeMode, isDirectPlay, isDirectStream]);

  // Transcode path — reload video with current audio track, quality and startTime
  const [transcodeStartTime, setTranscodeStartTime] = useState(startPositionMs / 1000);

  useEffect(() => {
    if (!transcodeMode) return;
    const video = videoRef.current;
    if (!video) return;

    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    const offset = transcodeStartTime;
    transcodeOffsetRef.current = offset;

    const token = accessToken ?? "";
    const params = new URLSearchParams();
    if (token) params.set("token", token);
    params.set("audioTrack", String(currentTrackIndex));
    if (currentQuality > 0) params.set("videoScale", String(currentQuality));
    if (canPlayHevc) params.set("remux", "1");
    if (offset > 0) params.set("startTime", String(offset));
    if (selectedBurnInTrack !== null) params.set("subtitleTrack", String(selectedBurnInTrack));
    const url = `/api/v1/media/${mediaId}/transcode?${params.toString()}`;

    let ready = false;
    const shouldPlay = !wasPausedRef.current;
    function onTimeUpdate() {
      positionRef.current = transcodeOffsetRef.current + video!.currentTime;
    }
    function handleCanPlay() { ready = true; }
    function handleEnded() { onEnded?.(); }
    function handleError() {
      const e = video!.error;
      if (e) {
        // Ignore errors before stream has loaded (stale events from previous src)
        if (!ready) return;
        // Ignore errors when paused (stream timeout is expected)
        if (video!.paused) return;
        // Ignore errors during grace period after a quality fallback
        if (Date.now() - lastFallbackTimeRef.current < 10000) return;
        console.warn("[player] transcode error:", e.code, e.message);
        const didFallback = attemptQualityFallbackRef.current(currentQualityRef.current);
        if (!didFallback) setFatalError(true);
      }
    }
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("canplay", handleCanPlay);
    video.addEventListener("ended", handleEnded);
    video.addEventListener("error", handleError);
    video.src = url;
    if (shouldPlay) video.play().catch(() => {});

    return () => {
      wasPausedRef.current = video.paused;
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("canplay", handleCanPlay);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("error", handleError);
      video.src = "";
    };
  }, [transcodeMode, mediaId, accessToken, onEnded, currentTrackIndex, currentQuality, transcodeStartTime, selectedBurnInTrack]);

  // Subtitle track management — imperatively add/remove <track> element
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    const existing = video.querySelector("track[data-dynamic]");
    if (existing) existing.remove();

    // Revoke previous blob URL if replaced
    if (externalSubUrlRef.current && externalSubUrlRef.current !== externalSubUrl?.url) {
      URL.revokeObjectURL(externalSubUrlRef.current);
      externalSubUrlRef.current = null;
    }

    // Don't create <track> for burn-in subtitles — they are rendered server-side in the video stream
    const isBurnIn = selectedBurnInTrack !== null;

    const src = externalSubUrl
      ? externalSubUrl.url
      : currentSubtitle !== null && !isBurnIn && accessToken
        ? `/api/v1/media/${mediaId}/subtitle/${currentSubtitle}?token=${accessToken}`
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

    // Force browser to load the track (dynamically added tracks aren't auto-loaded)
    if (track.track) track.track.mode = "showing";

    // Ensure all other text tracks are hidden
    for (let i = 0; i < video.textTracks.length; i++) {
      if (video.textTracks[i] !== track.track) {
        video.textTracks[i].mode = "hidden";
      }
    }
  }, [currentSubtitle, externalSubUrl, mediaId, accessToken, transcodeStartTime, currentTrackIndex, currentQuality]);

  // Apply subtitle position (top/bottom) via cue line setting
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

  // Inject ::cue style for subtitle appearance
  useEffect(() => {
    if (!styleElRef.current) {
      styleElRef.current = document.createElement("style");
      document.head.appendChild(styleElRef.current);
    }
    const { fontSize, color, fontFamily, bgColor, bgOpacity, shadow } = subtitleStyle;
    const fontFamilyRule = fontFamily ? `font-family: ${fontFamily};` : "";
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

  // Sync playbackRate with playbackSpeed state
  useEffect(() => {
    const video = videoRef.current;
    if (video) video.playbackRate = playbackSpeed;
  }, [playbackSpeed]);

  function showActionFlash(action: "play" | "pause") {
    if (actionFlashTimer.current) clearTimeout(actionFlashTimer.current);
    setActionFlash(action);
    actionFlashTimer.current = setTimeout(() => setActionFlash(null), 500);
  }

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      const video = videoRef.current;
      if (!video) return;

      switch (e.key) {
        case "Escape":
          e.preventDefault();
          if (anyPanelOpenRef.current) {
            closePanelRef.current?.();
          } else {
            onCloseRef.current?.();
          }
          break;
        case " ":
        case "k":
          e.preventDefault();
          if (video.paused) {
            video.play().catch(() => {});
            showActionFlash("play");
          } else {
            video.pause();
            showActionFlash("pause");
          }
          break;
        case "ArrowLeft":
        case "j":
          e.preventDefault();
          seekRef.current(Math.max(0, positionRef.current - skipBwdRef.current));
          break;
        case "ArrowRight":
        case "l": {
          e.preventDefault();
          const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
          seekRef.current(Math.min(dur || video.currentTime + skipFwdRef.current, positionRef.current + skipFwdRef.current));
          break;
        }
        case "ArrowUp":
          e.preventDefault();
          video.volume = Math.min(1, video.volume + 0.1);
          break;
        case "ArrowDown":
          e.preventDefault();
          video.volume = Math.max(0, video.volume - 0.1);
          break;
        case "m":
          video.muted = !video.muted;
          break;
        case "f": {
          const container = video.parentElement;
          if (!container) break;
          if (!document.fullscreenElement) void container.requestFullscreen();
          else void document.exitFullscreen();
          break;
        }
        case "c": {
          const subs = subtitleStreamsRef.current;
          const cur = currentSubtitleRef.current;
          if (subs.length === 0) break;
          if (cur === null) setCurrentSubtitle(subs[0].trackIndex);
          else setCurrentSubtitle(null);
          break;
        }
        case "p":
          if (document.pictureInPictureEnabled) {
            if (document.pictureInPictureElement) void document.exitPictureInPicture();
            else void video.requestPictureInPicture();
          }
          break;
        case "?":
          e.preventDefault();
          e.stopPropagation();
          setShowShortcuts((p) => !p);
          break;
        case "<":
        case ",": {
          e.preventDefault();
          const curSpeed = playbackSpeedRef.current;
          const idx = SPEED_OPTIONS.indexOf(curSpeed);
          if (idx > 0) setPlaybackSpeed(SPEED_OPTIONS[idx - 1]);
          break;
        }
        case ">":
        case ".": {
          e.preventDefault();
          const curSpeed = playbackSpeedRef.current;
          const idx = SPEED_OPTIONS.indexOf(curSpeed);
          if (idx < SPEED_OPTIONS.length - 1) setPlaybackSpeed(SPEED_OPTIONS[idx + 1]);
          break;
        }
        case "0": case "1": case "2": case "3": case "4":
        case "5": case "6": case "7": case "8": case "9": {
          e.preventDefault();
          const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
          if (dur > 0) seekRef.current(dur * (Number(e.key) / 10));
          break;
        }
      }
    }

    window.addEventListener("keydown", handleKeyDown, true);
    return () => window.removeEventListener("keydown", handleKeyDown, true);
  }, []);

  // Track time for next-episode overlay (absolute position)
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    function onTime() {
      const v = videoRef.current;
      if (!v) return;
      const absTime = transcodeOffsetRef.current + v.currentTime;
      setOverlayTime(absTime);
      const d = knownDurationSecRef.current;
      if (d > 0) setOverlayDuration(d);
      else {
        const vd = v.duration;
        if (vd && isFinite(vd) && vd > 0) setOverlayDuration(transcodeOffsetRef.current + vd);
      }
    }
    video.addEventListener("timeupdate", onTime);
    return () => video.removeEventListener("timeupdate", onTime);
  }, []);

  // Adaptive quality: measure buffer fill rate and adjust quality dynamically
  const slowCountRef = useRef(0);
  const lastBufferCheckRef = useRef({ time: 0, buffered: 0 });
  const qualityStableRef = useRef(0);

  useEffect(() => {
    if (!transcodeMode || !autoQuality) {
      slowCountRef.current = 0;
      lastBufferCheckRef.current = { time: 0, buffered: 0 };
      qualityStableRef.current = 0;
      return;
    }
    const video = videoRef.current;
    if (!video) return;

    lastBufferCheckRef.current = { time: Date.now(), buffered: 0 };
    qualityStableRef.current = Date.now();

    const interval = setInterval(() => {
      const v = videoRef.current;
      if (!v || v.paused) return;
      const b = v.buffered;
      if (b.length === 0) return;

      const now = Date.now();
      const currentBuffered = b.end(b.length - 1);
      const prev = lastBufferCheckRef.current;
      const elapsed = (now - prev.time) / 1000;
      lastBufferCheckRef.current = { time: now, buffered: currentBuffered };

      if (elapsed < 3) return;
      // Grace period after quality change — transcode needs startup time
      if (now - qualityStableRef.current < 15000) return;
      const fillRate = (currentBuffered - prev.buffered) / elapsed;

      const curIdx = QUALITY_LEVELS.indexOf(currentQuality);

      // Don't downgrade if buffer has enough headroom (>2min ahead of playback)
      const bufferAhead = currentBuffered - v.currentTime;
      // Only auto-adjust quality if we're already transcoding video.
      // If currentQuality === 0 (original), the server does video copy — downgrading
      // would force a full video transcode which is slower, causing a death spiral.
      if (currentQuality === 0) {
        slowCountRef.current = 0;
        return;
      }

      if (fillRate < 0.8 && bufferAhead < 120) {
        slowCountRef.current++;
        if (slowCountRef.current >= 2 && curIdx < QUALITY_LEVELS.length - 1) {
          const pos = positionRef.current;
          setCurrentQuality(QUALITY_LEVELS[curIdx + 1]);
          setTranscodeStartTime(pos);
          slowCountRef.current = 0;
          qualityStableRef.current = Date.now();
        }
      } else {
        slowCountRef.current = 0;
        if (fillRate > 2.5 && curIdx > 0 && (now - qualityStableRef.current) > 30000) {
          const pos = positionRef.current;
          setCurrentQuality(QUALITY_LEVELS[curIdx - 1]);
          setTranscodeStartTime(pos);
          qualityStableRef.current = Date.now();
        }
      }
    }, 5000);

    return () => clearInterval(interval);
  }, [transcodeMode, autoQuality, currentQuality]);

  const autoQualityLabel = autoQuality
    ? (currentQuality === 0 ? "Original" : `${currentQuality}p`)
    : undefined;

  const nextEpisode = (() => {
    if (!episodes || !currentEpisodeId) return null;
    const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
    if (idx < 0 || idx >= episodes.length - 1) return null;
    return episodes[idx + 1];
  })();

  const handleNextEpisode = useCallback((id: string) => {
    onEpisodeChange?.(id);
  }, [onEpisodeChange]);

  const handleSeekRequest = useCallback((absoluteTimeSec: number) => {
    const video = videoRef.current;
    if (!video || !transcodeMode) {
      if (video) video.currentTime = absoluteTimeSec;
      return;
    }
    const offset = transcodeOffsetRef.current;
    const relativeTime = absoluteTimeSec - offset;
    const buffered = video.buffered;
    const isBuffered = buffered.length > 0 && relativeTime >= 0 && relativeTime <= buffered.end(buffered.length - 1) + 2;
    if (isBuffered) {
      video.currentTime = relativeTime;
    } else {
      setTranscodeStartTime(absoluteTimeSec);
    }
  }, [transcodeMode]);

  const seekRef = useRef(handleSeekRequest);
  seekRef.current = handleSeekRequest;

  function attemptQualityFallback(currentQualityVal: number): boolean {
    // Don't downgrade from original — it would force a full video transcode
    if (currentQualityVal === 0) return false;
    if (fallbackCountRef.current >= MAX_FALLBACK_ATTEMPTS) return false;
    const curIdx = QUALITY_LEVELS.indexOf(currentQualityVal);
    if (curIdx < 0 || curIdx >= QUALITY_LEVELS.length - 1) return false;
    const nextQuality = QUALITY_LEVELS[curIdx + 1];
    fallbackCountRef.current++;
    lastFallbackTimeRef.current = Date.now();
    const qualityLabel = nextQuality === 0 ? "Original" : `${nextQuality}p`;
    console.warn(`[player] fatal error — downgrading to ${qualityLabel} (attempt ${fallbackCountRef.current})`);
    toast.warning(`Quality reduced to ${qualityLabel} due to a network error`);
    setCurrentQuality(nextQuality);
    setAutoQuality(false);
    setTranscodeStartTime(positionRef.current);
    return true;
  }
  attemptQualityFallbackRef.current = attemptQualityFallback;

  function handleQualityChange(quality: number) {
    setAutoQuality(false);
    setCurrentQuality(quality);
    if (transcodeMode) setTranscodeStartTime(positionRef.current);
  }

  function handleAutoQualityToggle(auto: boolean) {
    setAutoQuality(auto);
  }

  function handleAudioChange(index: number) {
    const stream = audioStreams.find((s) => s.index === index);
    setCurrentAudioIndex(index);
    if (stream) {
      setCurrentTrackIndex(stream.trackIndex);
      const lang = stream.language ?? stream.title ?? null;
      setPreferredAudioLang(lang);
      if (seriesId && lang) {
        const cur = getSeriesTrackPrefs(seriesId) ?? {};
        setSeriesTrackPrefs(seriesId, { ...cur, audioLang: lang });
      }
    }

    if (!transcodeMode) {
      if (hlsRef.current) {
        hlsRef.current.audioTrack = stream?.trackIndex ?? 0;
        return;
      }
      const video = videoRef.current as HTMLVideoElementWithAudioTracks | null;
      if (!video) return;
      const tracks = video.audioTracks;
      if (!tracks) return;
      for (let i = 0; i < tracks.length; i++) {
        tracks[i].enabled = tracks[i].id === String(index);
      }
    } else {
      setTranscodeStartTime(positionRef.current);
    }
  }

  // Touch gesture state: single-tap play/pause, double-tap to seek, swipe volume
  const touchStartRef = useRef<{ x: number; y: number; time: number } | null>(null);
  const lastTapRef = useRef<{ x: number; time: number } | null>(null);
  const singleTapTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [seekFlash, setSeekFlash] = useState<"left" | "right" | null>(null);
  const seekFlashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [actionFlash, setActionFlash] = useState<"play" | "pause" | null>(null);
  const actionFlashTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  function handleTouchStart(e: React.TouchEvent) {
    const t = e.touches[0];
    touchStartRef.current = { x: t.clientX, y: t.clientY, time: Date.now() };
  }

  function handleTouchEnd(e: React.TouchEvent) {
    const start = touchStartRef.current;
    if (!start) return;
    touchStartRef.current = null;
    e.preventDefault();
    const t = e.changedTouches[0];
    const dx = t.clientX - start.x;
    const dy = t.clientY - start.y;
    const dt = Date.now() - start.time;

    // Swipe up/down on right side → volume
    const el = e.currentTarget as HTMLElement;
    const rect = el.getBoundingClientRect();
    const isRightSide = t.clientX > rect.width * 0.6;
    if (Math.abs(dy) > 30 && Math.abs(dy) > Math.abs(dx) && dt < 500) {
      if (isRightSide) {
        const video = videoRef.current;
        if (video) {
          video.volume = Math.max(0, Math.min(1, video.volume + (dy < 0 ? 0.1 : -0.1)));
        }
        return;
      }
    }

    // Double-tap to seek ±10s
    const isSmallMove = Math.abs(dx) < 20 && Math.abs(dy) < 20;
    if (!isSmallMove || dt > 300) {
      lastTapRef.current = null;
      return;
    }
    const now = Date.now();
    const last = lastTapRef.current;
    if (last && now - last.time < 400) {
      if (singleTapTimer.current) { clearTimeout(singleTapTimer.current); singleTapTimer.current = null; }
      const isLeft = t.clientX < rect.width / 2;
      lastTapRef.current = null;
      const pos = positionRef.current;
      const video = videoRef.current;
      if (!video) return;
      const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
      const target = isLeft
        ? Math.max(0, pos - skipBwdRef.current)
        : Math.min(dur || pos + skipFwdRef.current, pos + skipFwdRef.current);
      seekRef.current(target);
      setSeekFlash(isLeft ? "left" : "right");
      if (seekFlashTimerRef.current) clearTimeout(seekFlashTimerRef.current);
      seekFlashTimerRef.current = setTimeout(() => setSeekFlash(null), 600);
    } else {
      lastTapRef.current = { x: t.clientX, time: now };
      // Single tap: toggle play/pause after delay (cancelled if double-tap follows)
      if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
      singleTapTimer.current = setTimeout(() => {
        singleTapTimer.current = null;
        const video = videoRef.current;
        if (!video) return;
        if (video.paused) { video.play().catch(() => {}); showActionFlash("play"); }
        else { video.pause(); showActionFlash("pause"); }
      }, 300);
    }
  }

  // Cleanup timers on unmount
  useEffect(() => {
    return () => {
      if (seekFlashTimerRef.current) clearTimeout(seekFlashTimerRef.current);
      if (actionFlashTimer.current) clearTimeout(actionFlashTimer.current);
      if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
      if (externalSubUrlRef.current) URL.revokeObjectURL(externalSubUrlRef.current);
    };
  }, []);

  return (
    <div
      className="absolute inset-0 bg-black"
      onTouchStart={handleTouchStart}
      onTouchEnd={handleTouchEnd}
    >
      <video ref={(el) => { videoRef.current = el; onVideoRef?.(el); }} className="w-full h-full" playsInline />
      {/* Touch seek flash indicators */}
      {seekFlash === "left" && (
        <div className="absolute left-0 top-0 bottom-0 w-1/2 flex items-center justify-center pointer-events-none" style={{ zIndex: 6 }}>
          <div className="rounded-full bg-white/20 w-16 h-16 flex items-center justify-center">
            <svg className="w-8 h-8 text-white" fill="currentColor" viewBox="0 0 24 24">
              <path d="M11.99 5V1l-5 5 5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6h-2c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"/>
            </svg>
          </div>
        </div>
      )}
      {seekFlash === "right" && (
        <div className="absolute right-0 top-0 bottom-0 w-1/2 flex items-center justify-center pointer-events-none" style={{ zIndex: 6 }}>
          <div className="rounded-full bg-white/20 w-16 h-16 flex items-center justify-center">
            <svg className="w-8 h-8 text-white" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12.01 5V1l5 5-5 5V7c-3.31 0-6 2.69-6 6s2.69 6 6 6 6-2.69 6-6h2c0 4.42-3.58 8-8 8s-8-3.58-8-8 3.58-8 8-8z"/>
            </svg>
          </div>
        </div>
      )}
      {/* Play/pause action flash (YouTube-style) */}
      {actionFlash && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none" style={{ zIndex: 6 }}>
          <div className="rounded-full bg-black/40 w-16 h-16 flex items-center justify-center animate-ping-once">
            {actionFlash === "play" ? (
              <svg className="w-8 h-8 text-white ml-1" fill="currentColor" viewBox="0 0 24 24">
                <path d="M5 3l14 9-14 9V3z" />
              </svg>
            ) : (
              <svg className="w-8 h-8 text-white" fill="currentColor" viewBox="0 0 24 24">
                <rect x="6" y="4" width="4" height="16" />
                <rect x="14" y="4" width="4" height="16" />
              </svg>
            )}
          </div>
        </div>
      )}
      {isBuffering && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none" style={{ zIndex: 5 }}>
          <div className="flex flex-col items-center gap-2">
            <svg className="animate-spin w-10 h-10 text-white/80" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
            </svg>
            <span className="text-white/80 text-sm font-medium">Buffering…</span>
          </div>
        </div>
      )}
      {fatalError && (
        <div className="absolute inset-0 flex items-center justify-center" style={{ zIndex: 10, background: "rgba(0,0,0,0.7)" }}>
          <div className="flex flex-col items-center gap-4 text-white">
            <svg className="w-12 h-12" style={{ color: "var(--color-error, #f87171)" }} fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
            </svg>
            <p className="text-lg font-medium">Playback failed</p>
            <p className="text-sm" style={{ color: "var(--text-muted, rgba(255,255,255,0.6))" }}>Unable to play at any quality level</p>
            <button
              onClick={() => { setFatalError(false); fallbackCountRef.current = 0; window.location.reload(); }}
              className="px-4 py-2 rounded text-sm font-medium"
              style={{ backgroundColor: "rgba(255,255,255,0.25)" }}
            >
              Retry
            </button>
          </div>
        </div>
      )}
      <ProgressReporter sessionId={sessionId} videoRef={videoRef} transcodeOffsetRef={transcodeOffsetRef} />
      <SkipIntroButton
        filePath={filePath}
        currentTime={overlayTime}
        onSkip={(t) => handleSeekRequest(t)}
      />
      {nextEpisode && (
        <NextEpisodeOverlay
          nextEpisode={nextEpisode}
          currentTime={overlayTime}
          duration={overlayDuration}
          onPlayNow={handleNextEpisode}
          onCancel={() => {}}
        />
      )}
      <PlayerControls
        videoRef={videoRef}
        knownDurationSec={knownDurationMs ? knownDurationMs / 1000 : undefined}
        audioStreams={audioStreams}
        subtitleStreams={subtitleStreams}
        title={title}
        onClose={onClose}
        currentAudioIndex={currentAudioIndex}
        onAudioChange={handleAudioChange}
        currentQuality={currentQuality}
        onQualityChange={handleQualityChange}
        autoQuality={autoQuality}
        onAutoQualityToggle={handleAutoQualityToggle}
        autoQualityLabel={autoQualityLabel}
        currentSubtitleTrackIndex={currentSubtitle}
        onSubtitleChange={(trackIndex) => {
          const prevBurnIn = selectedBurnInTrack;
          const nextSub = trackIndex !== null ? subtitleStreams.find((st) => st.trackIndex === trackIndex) : null;
          const nextBurnIn = nextSub?.burnIn ? trackIndex : null;

          setCurrentSubtitle(trackIndex);

          // Restart transcode when switching between burn-in tracks or toggling burn-in on/off
          if (transcodeMode && prevBurnIn !== nextBurnIn) {
            setTranscodeStartTime(positionRef.current);
          }

          if (trackIndex === null) {
            setPreferredSubtitleLang(null);
            if (seriesId) {
              const cur = getSeriesTrackPrefs(seriesId) ?? {};
              setSeriesTrackPrefs(seriesId, { ...cur, subtitleLang: null });
            }
          } else {
            const s = subtitleStreams.find((st) => st.trackIndex === trackIndex);
            const lang = s?.language ?? s?.title ?? null;
            setPreferredSubtitleLang(lang);
            if (seriesId) {
              const cur = getSeriesTrackPrefs(seriesId) ?? {};
              setSeriesTrackPrefs(seriesId, { ...cur, subtitleLang: lang });
            }
          }
        }}
        playbackSpeed={playbackSpeed}
        onSpeedChange={setPlaybackSpeed}
        seasons={seasons}
        currentSeasonId={currentSeasonId}
        episodes={episodes}
        currentEpisodeId={currentEpisodeId}
        onSeasonChange={onSeasonChange}
        onEpisodeChange={onEpisodeChange}
        chapters={chapters}
        mediaId={mediaId}
        onSeekRequest={handleSeekRequest}
        transcodeOffset={transcodeOffsetRef.current}
        skipForwardSeconds={skipFwd}
        skipBackwardSeconds={skipBwd}
        subtitleStyle={subtitleStyle}
        onSubtitleStyleChange={updateSubtitleStyle}
        onExternalSubtitle={(blobUrl, label) => {
          setCurrentSubtitle(null);
          setExternalSubUrl({ url: blobUrl, label });
        }}
        onPanelOpenChange={(anyOpen) => { anyPanelOpenRef.current = anyOpen; }}
        closePanelRef={closePanelRef}
        onActionFlash={showActionFlash}
      />
      {showShortcuts && <KeyboardShortcutsOverlay onClose={() => setShowShortcuts(false)} skipForwardSeconds={skipFwd} skipBackwardSeconds={skipBwd} />}
    </div>
  );
}
