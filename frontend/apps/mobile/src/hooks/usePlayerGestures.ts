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
}

export interface GestureOverlayState {
  seekFlash: "left" | "right" | null;
  volumeIndicator: number | null;
  brightnessIndicator: number | null;
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

  const flashSeek = useCallback((side: "left" | "right") => {
    setSeekFlash(side);
    if (seekFlashTimer.current) clearTimeout(seekFlashTimer.current);
    seekFlashTimer.current = setTimeout(() => setSeekFlash(null), 600);
  }, []);

  const panResponder = useMemo(
    () =>
      PanResponder.create({
        onStartShouldSetPanResponder: () => true,
        onMoveShouldSetPanResponder: (_, gs) =>
          Math.abs(gs.dy) > SWIPE_THRESHOLD && Math.abs(gs.dy) > Math.abs(gs.dx) * 1.2,

        onPanResponderGrant: () => {
          swipeActiveRef.current = null;
          swipeStartVolumeRef.current = getVolume();
          swipeStartBrightnessRef.current = getBrightness();
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
            const newVol = Math.max(0, Math.min(1, swipeStartVolumeRef.current + dy * SENSITIVITY));
            onVolumeChange(newVol);
            setVolumeIndicator(newVol);
          } else if (swipeActiveRef.current === "brightness") {
            const newBri = Math.max(0, Math.min(1, swipeStartBrightnessRef.current + dy * SENSITIVITY));
            onBrightnessChange(newBri);
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
              onSeekBy(-skipBackwardSeconds);
              flashSeek("left");
            } else if (relX > RIGHT_ZONE) {
              onSeekBy(skipForwardSeconds);
              flashSeek("right");
            } else {
              onTogglePlayPause();
            }
          } else {
            lastTapRef.current = { x: tapX, time: now };
            if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
            singleTapTimer.current = setTimeout(() => {
              singleTapTimer.current = null;
              onToggleControls();
            }, DOUBLE_TAP_DELAY);
          }
        },
      }),
    [onSeekBy, onTogglePlayPause, onToggleControls, onVolumeChange, onBrightnessChange, getVolume, getBrightness, flashSeek, skipForwardSeconds, skipBackwardSeconds],
  );

  const cleanup = useCallback(() => {
    if (seekFlashTimer.current) clearTimeout(seekFlashTimer.current);
    if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
  }, []);

  return { panResponder, seekFlash, volumeIndicator, brightnessIndicator, cleanup };
}
