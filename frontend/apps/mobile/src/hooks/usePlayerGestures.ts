import { useRef, useState, useCallback, useMemo } from "react";
import { PanResponder, Dimensions } from "react-native";
import type { GestureResponderEvent, PanResponderGestureState } from "react-native";

const DOUBLE_TAP_DELAY = 300;
const SWIPE_THRESHOLD = 15;
const SENSITIVITY = 0.004;
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
          Math.abs(gs.dy) > SWIPE_THRESHOLD && Math.abs(gs.dy) > Math.abs(gs.dx),

        onPanResponderGrant: () => {
          swipeActiveRef.current = null;
          swipeStartVolumeRef.current = getVolume();
          swipeStartBrightnessRef.current = getBrightness();
        },

        onPanResponderMove: (evt: GestureResponderEvent, gs: PanResponderGestureState) => {
          const { width } = Dimensions.get("screen");
          const relX = evt.nativeEvent.pageX / width;

          if (!swipeActiveRef.current) {
            if (Math.abs(gs.dy) < SWIPE_THRESHOLD) return;
            if (Math.abs(gs.dx) > Math.abs(gs.dy)) return;
            if (relX > 0.6) {
              swipeActiveRef.current = "volume";
            } else if (relX < 0.4) {
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

          const { width } = Dimensions.get("screen");
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
              onSeekBy(-10);
              flashSeek("left");
            } else if (relX > RIGHT_ZONE) {
              onSeekBy(10);
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
    [onSeekBy, onTogglePlayPause, onToggleControls, onVolumeChange, onBrightnessChange, getVolume, getBrightness, flashSeek],
  );

  const cleanup = useCallback(() => {
    if (seekFlashTimer.current) clearTimeout(seekFlashTimer.current);
    if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
  }, []);

  return { panResponder, seekFlash, volumeIndicator, brightnessIndicator, cleanup };
}
