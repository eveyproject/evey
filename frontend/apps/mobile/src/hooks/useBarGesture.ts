import { useCallback, useRef } from "react";
import type { GestureResponderEvent, LayoutChangeEvent, View } from "react-native";

/**
 * Touch handling for a horizontal bar: the finger as a 0-1 fraction of it.
 * pageX against the bar's own window position, never locationX: on Android
 * that one is relative to the child the finger landed on (thumb, fill), as
 * measured on the seek bar (580d67890). The bar keeps the gesture: the player
 * stage's pan responder asks for every horizontal drag past 16 dp.
 */
export function useBarGesture(onFraction: (fraction: number) => void) {
  const ref = useRef<View>(null);
  const pageXRef = useRef(0);
  const widthRef = useRef(0);
  const onFractionRef = useRef(onFraction);
  onFractionRef.current = onFraction;

  const measure = useCallback(() => {
    ref.current?.measureInWindow((x, _y, width) => {
      pageXRef.current = x;
      if (width > 0) widthRef.current = width;
    });
  }, []);
  // The finger's fraction, or null before the bar has been measured.
  const fractionAt = useCallback((e: GestureResponderEvent): number | null => {
    const w = widthRef.current;
    const pageX = e?.nativeEvent?.pageX;
    if (w <= 0 || typeof pageX !== "number") return null;
    return Math.max(0, Math.min(1, (pageX - pageXRef.current) / w));
  }, []);
  const emit = useCallback((e: GestureResponderEvent) => {
    const f = fractionAt(e);
    if (f !== null) onFractionRef.current(f);
  }, [fractionAt]);

  const barProps = {
    ref,
    onLayout: (e: LayoutChangeEvent) => { widthRef.current = e.nativeEvent.layout.width; measure(); },
    onStartShouldSetResponder: () => true,
    onMoveShouldSetResponder: () => true,
    // Re-measured on every grant: the panel or the controls may have moved.
    onResponderGrant: (e: GestureResponderEvent) => { measure(); emit(e); },
    onResponderMove: emit,
    onResponderTerminationRequest: () => false,
  };
  return { barProps, fractionAt };
}
