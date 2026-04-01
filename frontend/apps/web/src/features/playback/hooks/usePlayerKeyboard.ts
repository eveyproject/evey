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
  onCloseRef: React.MutableRefObject<(() => void) | undefined>;
  setPlaybackSpeed: (s: number) => void;
  setCurrentSubtitle: (v: number | null) => void;
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
  onCloseRef,
  setPlaybackSpeed,
  setCurrentSubtitle,
  setShowShortcuts,
  showActionFlash,
}: UsePlayerKeyboardArgs) {
  const setPlaybackSpeedRef = useRef(setPlaybackSpeed);
  setPlaybackSpeedRef.current = setPlaybackSpeed;
  const setCurrentSubtitleRef = useRef(setCurrentSubtitle);
  setCurrentSubtitleRef.current = setCurrentSubtitle;
  const setShowShortcutsRef = useRef(setShowShortcuts);
  setShowShortcutsRef.current = setShowShortcuts;
  const showActionFlashRef = useRef(showActionFlash);
  showActionFlashRef.current = showActionFlash;

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
            showActionFlashRef.current("play");
          } else {
            video.pause();
            showActionFlashRef.current("pause");
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
          if (cur === null) setCurrentSubtitleRef.current(subs[0].trackIndex);
          else setCurrentSubtitleRef.current(null);
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
