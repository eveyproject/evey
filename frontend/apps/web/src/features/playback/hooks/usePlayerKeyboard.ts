import { useEffect, useRef } from "react";
import type { SubtitleStream } from "../SubtitleSelector";
import { SPEED_OPTIONS } from "../constants";

interface UsePlayerKeyboardArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  positionRef: React.MutableRefObject<number>;
  knownDurationSecRef: React.MutableRefObject<number>;
  seekRef: React.MutableRefObject<(absoluteTimeSec: number) => void>;
  skipFwdRef: React.MutableRefObject<number>;
  skipBwdRef: React.MutableRefObject<number>;
  playbackSpeedRef: React.MutableRefObject<number>;
  subtitleStreamsRef: React.MutableRefObject<SubtitleStream[]>;
  currentSubtitleRef: React.MutableRefObject<number | null>;
  anyPanelOpenRef: React.MutableRefObject<boolean>;
  closePanelRef: React.MutableRefObject<(() => void) | null>;
  // PlayerControls' Document-PiP-aware toggle. When present the "p" shortcut
  // uses it; otherwise it falls back to the legacy bare-<video> PiP.
  pipToggleRef: React.MutableRefObject<(() => void) | null>;
  // Live cast session, updated render-time by the host. While connected the
  // space/k toggle drives the TV, never the local video: the handoff paused
  // the local element, and waking it plays BOTH streams at once with no way
  // to pause the local one (its controls are cast-bound).
  castRef: React.MutableRefObject<{ connected: boolean; isPaused: boolean; playPause: () => void } | null>;
  onCloseRef: React.MutableRefObject<(() => void) | undefined>;
  // Reveals the auto-hidden control bars on a player keystroke. A hidden bar
  // now leaves layout (visibility, not just opacity), so a control clicked
  // with the mouse loses its focus when the bar fades and the bar's own
  // onKeyDown no longer sees the next key: the shortcut handler reveals
  // instead, as any key on a focused control used to.
  revealControlsRef?: React.MutableRefObject<(() => void) | null>;
  setPlaybackSpeed: (s: number) => void;
  // Burn-in-aware subtitle selector (same handler the settings panel uses), so
  // the "c" shortcut restarts the transcode for PGS/VobSub tracks instead of
  // silently flipping state with no on-screen subtitle.
  onSubtitleChange: (trackIndex: number | null) => void;
  setShowShortcuts: (fn: (prev: boolean) => boolean) => void;
  showActionFlash: (action: "play" | "pause") => void;
}

export function usePlayerKeyboard({
  videoRef,
  positionRef,
  knownDurationSecRef,
  seekRef,
  skipFwdRef,
  skipBwdRef,
  playbackSpeedRef,
  subtitleStreamsRef,
  currentSubtitleRef,
  anyPanelOpenRef,
  closePanelRef,
  pipToggleRef,
  castRef,
  onCloseRef,
  setPlaybackSpeed,
  onSubtitleChange,
  setShowShortcuts,
  showActionFlash,
  revealControlsRef,
}: UsePlayerKeyboardArgs) {
  const setPlaybackSpeedRef = useRef(setPlaybackSpeed);
  setPlaybackSpeedRef.current = setPlaybackSpeed;
  const onSubtitleChangeRef = useRef(onSubtitleChange);
  onSubtitleChangeRef.current = onSubtitleChange;
  const setShowShortcutsRef = useRef(setShowShortcuts);
  setShowShortcutsRef.current = setShowShortcuts;
  const showActionFlashRef = useRef(showActionFlash);
  showActionFlashRef.current = showActionFlash;

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      const video = videoRef.current;
      if (!video) return;

      // Yield to any modal dialog open over the player (share, shortcuts help,
      // parental gate). It owns the keyboard - most importantly Escape, which
      // must close the dialog, not exit the player. This listener is on
      // window-capture and would otherwise fire before the dialog's own handler.
      if (document.querySelector('[role="dialog"][aria-modal="true"]')) return;

      // Don't hijack typing: if focus is in a text field (search box,
      // settings input, contentEditable), let the keystroke through instead
      // of treating "k"/"l"/"j"/space as player shortcuts.
      const target = e.target as HTMLElement | null;
      if (target) {
        const tag = target.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || target.isContentEditable) {
          return;
        }
      }

      revealControlsRef?.current?.();

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
        case "k": {
          e.preventDefault();
          const cast = castRef.current;
          if (cast?.connected) {
            cast.playPause();
            showActionFlashRef.current(cast.isPaused ? "play" : "pause");
            break;
          }
          if (video.paused) {
            video.play().catch((err) => console.warn("video.play failed", err));
            showActionFlashRef.current("play");
          } else {
            video.pause();
            showActionFlashRef.current("pause");
          }
          break;
        }
        case "ArrowLeft":
        case "j":
          e.preventDefault();
          seekRef.current(Math.max(0, positionRef.current - skipBwdRef.current));
          break;
        case "ArrowRight":
        case "l": {
          e.preventDefault();
          const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
          // Target is the ABSOLUTE position + skip; clamp to duration only
          // when it's known. The previous `dur || (currentTime + skip)`
          // fallback used a LOCAL value as the ceiling, which in transcode
          // mode (positionRef absolute > local) seeked backwards.
          const fwdTarget = positionRef.current + skipFwdRef.current;
          seekRef.current(dur > 0 ? Math.min(dur, fwdTarget) : fwdTarget);
          break;
        }
        case "ArrowUp": {
          e.preventDefault();
          const next = Math.min(1, video.volume + 0.1);
          video.volume = next;
          // Same intent as the volume slider: a deliberate raise clears the
          // muted-autoplay flag so the user hears something instead of seeing
          // the volume bar move silently. Symmetric with the slider fix
          // (PlayerControls.handleVolume).
          if (next > 0 && video.muted) video.muted = false;
          break;
        }
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
          // Route through the burn-in-aware handler so PGS/VobSub tracks
          // trigger a transcode restart (otherwise nothing renders on screen).
          onSubtitleChangeRef.current(cur === null ? subs[0].trackIndex : null);
          break;
        }
        case "p":
          // Prefer the controls' Document-PiP-aware toggle (rich PiP window);
          // fall back to legacy bare-<video> PiP when controls aren't mounted.
          if (pipToggleRef.current) {
            pipToggleRef.current();
          } else if (document.pictureInPictureEnabled) {
            if (document.pictureInPictureElement) void document.exitPictureInPicture();
            else void video.requestPictureInPicture();
          }
          break;
        case "?":
          e.preventDefault();
          e.stopPropagation();
          setShowShortcutsRef.current((p: boolean) => !p);
          break;
        case "<":
        case ",": {
          e.preventDefault();
          const curSpeed = playbackSpeedRef.current;
          const idx = SPEED_OPTIONS.indexOf(curSpeed);
          if (idx > 0) setPlaybackSpeedRef.current(SPEED_OPTIONS[idx - 1]);
          break;
        }
        case ">":
        case ".": {
          e.preventDefault();
          const curSpeed = playbackSpeedRef.current;
          const idx = SPEED_OPTIONS.indexOf(curSpeed);
          if (idx < SPEED_OPTIONS.length - 1) setPlaybackSpeedRef.current(SPEED_OPTIONS[idx + 1]);
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
}
