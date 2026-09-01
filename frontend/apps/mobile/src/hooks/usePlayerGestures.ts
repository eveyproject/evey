import { useRef, useState, useCallback, useMemo } from "react";
import { PanResponder, Dimensions } from "react-native";
import type { GestureResponderEvent, PanResponderGestureState } from "react-native";

const DOUBLE_TAP_DELAY = 300;
// Lower threshold + tighter Y-vs-X ratio so vertical swipes are detected
// quickly, even when the user starts with a slight diagonal motion. Higher
// sensitivity makes 0..1 reachable with a half-screen swipe instead of
// requiring a full-screen drag.
const SWIPE_THRESHOLD = 8;
const SENSITIVITY = 0.0025;
const LEFT_ZONE = 0.35;
const RIGHT_ZONE = 0.65;
// Horizontal scrub. The threshold is higher than the vertical one on purpose:
// a double tap that drifts sideways must stay a double tap, and the tap path
// already tolerates 20px of slop.
const SCRUB_THRESHOLD = 16;
// How much timeline one full-width drag covers. Proportional to the runtime so
// the gesture feels the same on a 22-minute episode and a 3-hour film, clamped
// so a very short clip stays scrubbable and a long one does not turn every
// pixel into half a minute.
const SCRUB_SPAN_FRACTION = 0.1;
const SCRUB_SPAN_MIN_SEC = 60;
const SCRUB_SPAN_MAX_SEC = 600;

export function scrubSpanSeconds(durationSec: number): number {
  return Math.max(SCRUB_SPAN_MIN_SEC, Math.min(SCRUB_SPAN_MAX_SEC, durationSec * SCRUB_SPAN_FRACTION));
}

/** Live preview of a horizontal scrub: where the release would land, and by how
 *  much it moves. Null when no scrub is in progress. */
export interface ScrubPreview {
  targetSec: number;
  deltaSec: number;
  durationSec: number;
}

interface UsePlayerGesturesArgs {
  onSeekBy: (seconds: number) => void;
  /** Commit of a horizontal scrub: an ABSOLUTE position in seconds. Seeking
   *  once on release rather than on every move keeps a drag from hammering an
   *  HLS session with dozens of seeks. */
  onScrubCommit?: (targetSeconds: number) => void;
  getPosition?: () => number;
  getDuration?: () => number;
  onTogglePlayPause: () => void;
  onToggleControls: () => void;
  onVolumeChange: (absoluteValue: number) => void;
  onBrightnessChange: (absoluteValue: number) => void;
  getVolume: () => number;
  getBrightness: () => number;
  skipForwardSeconds?: number;
  skipBackwardSeconds?: number;
  /** True when a docked panel (settings...) is open: volume/brightness
   *  swipes are suspended (they would steal the panel's scroll) and a tap
   *  on the video closes the panel - same contract as the web player's
   *  overlay click. */
  hasOpenPanel?: () => boolean;
  onCloseOverlays?: () => void;
}

export function usePlayerGestures({
  onSeekBy,
  onScrubCommit,
  getPosition,
  getDuration,
  onTogglePlayPause,
  onToggleControls,
  onVolumeChange,
  onBrightnessChange,
  getVolume,
  getBrightness,
  skipForwardSeconds = 30,
  skipBackwardSeconds = 30,
  hasOpenPanel,
  onCloseOverlays,
}: UsePlayerGesturesArgs) {
  const [seekFlash, setSeekFlash] = useState<"left" | "right" | null>(null);
  const [scrubPreview, setScrubPreview] = useState<ScrubPreview | null>(null);
  const [volumeIndicator, setVolumeIndicator] = useState<number | null>(null);
  const [brightnessIndicator, setBrightnessIndicator] = useState<number | null>(null);
  const seekFlashTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastTapRef = useRef<{ x: number; time: number } | null>(null);
  const singleTapTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const swipeActiveRef = useRef<"volume" | "brightness" | "scrub" | null>(null);
  const swipeStartVolumeRef = useRef(1);
  const swipeStartBrightnessRef = useRef(1);
  const scrubStartPositionRef = useRef(0);
  const scrubDurationRef = useRef(0);
  // The move handler owns the target; the release handler commits it. Kept in a
  // ref rather than read back off `seekPreview`, whose setState has not
  // necessarily been applied by the time the finger lifts.
  const scrubTargetRef = useRef(0);

  // Latest callbacks/params in a ref so the PanResponder can be built ONCE with
  // stable panHandlers. Recreating it mid-swipe (which happened whenever one of
  // these changed identity on a setState re-render) re-granted the gesture and
  // reset gestureState.dy to ~0 - so volume/brightness barely moved ("stuck at
  // ~97% then snaps back").
  const cb = useRef({
    onSeekBy, onScrubCommit, getPosition, getDuration,
    onTogglePlayPause, onToggleControls, onVolumeChange,
    onBrightnessChange, getVolume, getBrightness, skipForwardSeconds, skipBackwardSeconds,
    hasOpenPanel, onCloseOverlays,
  });
  cb.current = {
    onSeekBy, onScrubCommit, getPosition, getDuration,
    onTogglePlayPause, onToggleControls, onVolumeChange,
    onBrightnessChange, getVolume, getBrightness, skipForwardSeconds, skipBackwardSeconds,
    hasOpenPanel, onCloseOverlays,
  };

  const flashSeek = useCallback((side: "left" | "right") => {
    setSeekFlash(side);
    if (seekFlashTimer.current) clearTimeout(seekFlashTimer.current);
    seekFlashTimer.current = setTimeout(() => setSeekFlash(null), 600);
  }, []);

  const panResponder = useMemo(
    () =>
      PanResponder.create({
        // Docked panel open: don't claim the responder at all, otherwise the
        // panel's ScrollView never receives moves (the scrim set by the
        // player screen handles closing on tap).
        onStartShouldSetPanResponder: () => !cb.current.hasOpenPanel?.(),
        onMoveShouldSetPanResponder: (_, gs) => {
          if (cb.current.hasOpenPanel?.()) return false;
          const vertical = Math.abs(gs.dy) > SWIPE_THRESHOLD && Math.abs(gs.dy) > Math.abs(gs.dx) * 1.2;
          const horizontal = Math.abs(gs.dx) > SCRUB_THRESHOLD && Math.abs(gs.dx) > Math.abs(gs.dy) * 1.2;
          return vertical || horizontal;
        },

        onPanResponderGrant: () => {
          swipeActiveRef.current = null;
          swipeStartVolumeRef.current = cb.current.getVolume();
          swipeStartBrightnessRef.current = cb.current.getBrightness();
          // Snapshot the timeline once: reading the live position on every move
          // would compound the player's own advance into the drag.
          scrubStartPositionRef.current = cb.current.getPosition?.() ?? 0;
          scrubDurationRef.current = cb.current.getDuration?.() ?? 0;
          scrubTargetRef.current = scrubStartPositionRef.current;
        },

        onPanResponderMove: (evt: GestureResponderEvent, gs: PanResponderGestureState) => {
          const { width: screenWidth, height: screenHeight } = Dimensions.get("window");
          // Pan handler attaches to the player view which fills the whole
          // window, so window width matches the visible touch surface in
          // both portrait and landscape (Dimensions.screen included system
          // bars and gave a wrong relX after the fullscreen lock).
          const longEdge = Math.max(screenWidth, screenHeight);
          const shortEdge = Math.min(screenWidth, screenHeight);
          // In landscape the X axis is the long edge; in portrait it's the
          // short edge. relX stays a 0..1 ratio either way.
          const xExtent = screenWidth >= screenHeight ? longEdge : shortEdge;
          const relX = Math.max(0, Math.min(1, evt.nativeEvent.pageX / xExtent));

          if (!swipeActiveRef.current) {
            // Horizontal wins first: a drag that is mostly sideways is a
            // timeline scrub wherever on the screen it started, so it must not
            // be classified by the left/right half the way a vertical one is.
            // Needs a known runtime - there is nothing to scrub along without
            // one (live, or metadata not in yet).
            if (Math.abs(gs.dx) > SCRUB_THRESHOLD && Math.abs(gs.dx) > Math.abs(gs.dy) * 1.2) {
              if (scrubDurationRef.current <= 0 || !cb.current.onScrubCommit) return;
              swipeActiveRef.current = "scrub";
              // Fall through rather than return: the move that CLASSIFIES the
              // gesture already carries a real dx, so the overlay must appear
              // on it. Returning here left the first frame of every drag blank.
            } else {
              if (Math.abs(gs.dy) < SWIPE_THRESHOLD) return;
              if (Math.abs(gs.dx) > Math.abs(gs.dy)) return;
              // Split the screen 50/50: right half = volume, left half =
              // brightness. The previous 0.4/0.6 dead-zone in the middle
              // ate too many genuinely centered swipes.
              if (relX >= 0.5) {
                swipeActiveRef.current = "volume";
              } else {
                swipeActiveRef.current = "brightness";
              }
              return;
            }
          }

          if (swipeActiveRef.current === "scrub") {
            const duration = scrubDurationRef.current;
            const span = scrubSpanSeconds(duration);
            const raw = scrubStartPositionRef.current + (gs.dx / xExtent) * span;
            const target = Math.max(0, Math.min(duration, raw));
            scrubTargetRef.current = target;
            setScrubPreview({
              targetSec: target,
              deltaSec: target - scrubStartPositionRef.current,
              durationSec: duration,
            });
            return;
          }

          const dy = -gs.dy; // positive = swipe up

          if (swipeActiveRef.current === "volume") {
            // 0-2 range: 0-1 rides the native player volume, 1-2 engages the
            // LoudnessEnhancer boost (see handleVolumeChange in the player screen).
            const newVol = Math.max(0, Math.min(2, swipeStartVolumeRef.current + dy * SENSITIVITY));
            cb.current.onVolumeChange(newVol);
            setVolumeIndicator(newVol);
          } else if (swipeActiveRef.current === "brightness") {
            const newBri = Math.max(0, Math.min(1, swipeStartBrightnessRef.current + dy * SENSITIVITY));
            cb.current.onBrightnessChange(newBri);
            setBrightnessIndicator(newBri);
          }
        },

        // A terminated gesture (the OS handing the responder to something else)
        // must not commit a seek, and must not leave the centred preview pill
        // frozen over the video.
        onPanResponderTerminate: () => {
          swipeActiveRef.current = null;
          setScrubPreview(null);
          setVolumeIndicator(null);
          setBrightnessIndicator(null);
        },

        onPanResponderRelease: (evt: GestureResponderEvent, gs: PanResponderGestureState) => {
          if (swipeActiveRef.current === "scrub") {
            swipeActiveRef.current = null;
            setScrubPreview(null);
            cb.current.onScrubCommit?.(scrubTargetRef.current);
            return;
          }
          if (swipeActiveRef.current) {
            swipeActiveRef.current = null;
            setVolumeIndicator(null);
            setBrightnessIndicator(null);
            return;
          }

          // Tap detection
          const isSmallMove = Math.abs(gs.dx) < 20 && Math.abs(gs.dy) < 20;
          if (!isSmallMove) return;

          // Panel open: the tap closes the panel, nothing else (web:
          // handleOverlayClick does exactly this before any play/pause).
          if (cb.current.hasOpenPanel?.()) {
            cb.current.onCloseOverlays?.();
            return;
          }

          const { width } = Dimensions.get("window");
          const tapX = evt.nativeEvent.pageX;
          const relX = tapX / width;
          const now = Date.now();
          const last = lastTapRef.current;

          if (last && now - last.time < DOUBLE_TAP_DELAY) {
            if (singleTapTimer.current) {
              clearTimeout(singleTapTimer.current);
              singleTapTimer.current = null;
            }
            lastTapRef.current = null;

            if (relX < LEFT_ZONE) {
              cb.current.onSeekBy(-cb.current.skipBackwardSeconds);
              flashSeek("left");
            } else if (relX > RIGHT_ZONE) {
              cb.current.onSeekBy(cb.current.skipForwardSeconds);
              flashSeek("right");
            } else {
              cb.current.onTogglePlayPause();
            }
          } else {
            lastTapRef.current = { x: tapX, time: now };
            if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
            singleTapTimer.current = setTimeout(() => {
              singleTapTimer.current = null;
              cb.current.onToggleControls();
            }, DOUBLE_TAP_DELAY);
          }
        },
      }),
    // Built ONCE - all callbacks/params are read through the `cb` ref (see above),
    // so the panHandlers stay stable and a swipe is never re-granted mid-gesture.
    [flashSeek],
  );

  const cleanup = useCallback(() => {
    if (seekFlashTimer.current) clearTimeout(seekFlashTimer.current);
    if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
  }, []);

  return { panResponder, seekFlash, scrubPreview, volumeIndicator, brightnessIndicator, cleanup };
}
