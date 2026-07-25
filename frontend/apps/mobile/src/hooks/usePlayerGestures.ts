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

interface UsePlayerGesturesArgs {
  onSeekBy: (seconds: number) => void;
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
  const [volumeIndicator, setVolumeIndicator] = useState<number | null>(null);
  const [brightnessIndicator, setBrightnessIndicator] = useState<number | null>(null);
  const seekFlashTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastTapRef = useRef<{ x: number; time: number } | null>(null);
  const singleTapTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const swipeActiveRef = useRef<"volume" | "brightness" | null>(null);
  const swipeStartVolumeRef = useRef(1);
  const swipeStartBrightnessRef = useRef(1);

  // Latest callbacks/params in a ref so the PanResponder can be built ONCE with
  // stable panHandlers. Recreating it mid-swipe (which happened whenever one of
  // these changed identity on a setState re-render) re-granted the gesture and
  // reset gestureState.dy to ~0 - so volume/brightness barely moved ("stuck at
  // ~97% then snaps back").
  const cb = useRef({
    onSeekBy, onTogglePlayPause, onToggleControls, onVolumeChange,
    onBrightnessChange, getVolume, getBrightness, skipForwardSeconds, skipBackwardSeconds,
    hasOpenPanel, onCloseOverlays,
  });
  cb.current = {
    onSeekBy, onTogglePlayPause, onToggleControls, onVolumeChange,
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
        onMoveShouldSetPanResponder: (_, gs) =>
          !cb.current.hasOpenPanel?.() &&
          Math.abs(gs.dy) > SWIPE_THRESHOLD && Math.abs(gs.dy) > Math.abs(gs.dx) * 1.2,

        onPanResponderGrant: () => {
          swipeActiveRef.current = null;
          swipeStartVolumeRef.current = cb.current.getVolume();
          swipeStartBrightnessRef.current = cb.current.getBrightness();
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

        onPanResponderRelease: (evt: GestureResponderEvent, gs: PanResponderGestureState) => {
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

  return { panResponder, seekFlash, volumeIndicator, brightnessIndicator, cleanup };
}
