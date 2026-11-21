import { useCallback, useEffect, useRef, useState } from "react";
import type { LayoutChangeEvent, ScrollView, ViewStyle } from "react-native";

const GAP = 24;
const LEAVE_DELAY_MS = 150;

// A television keyboard floats over the bottom of the screen and reports no
// height, so the window is never resized and nothing scrolls on its own.
// While a field is being edited the form is laid out from the top and scrolled
// until its first field sits under the top edge, clear of any keyboard.
export function useFieldAboveKeyboard(editing: boolean) {
  const scrollRef = useRef<ScrollView>(null);
  const [active, setActive] = useState(false);
  const [viewport, setViewport] = useState(0);
  const [fieldTop, setFieldTop] = useState(0);

  // Moving from one field to the next blurs before it focuses: leaving is
  // delayed so the form does not bounce back to the centre in between.
  useEffect(() => {
    if (editing) {
      setActive(true);
      return;
    }
    const timer = setTimeout(() => setActive(false), LEAVE_DELAY_MS);
    return () => clearTimeout(timer);
  }, [editing]);

  const target = active ? Math.max(0, fieldTop - GAP) : 0;
  const scrollToTarget = useCallback(() => {
    scrollRef.current?.scrollTo?.({ y: target, animated: false });
  }, [target]);
  useEffect(scrollToTarget, [scrollToTarget]);

  const onViewportLayout = useCallback((e: LayoutChangeEvent) => setViewport(e.nativeEvent.layout.height), []);
  const onFieldLayout = useCallback((e: LayoutChangeEvent) => setFieldTop(e.nativeEvent.layout.y), []);
  // The extra padding is what gives the scroll view the room to bring the
  // field up when the form is shorter than the screen.
  const contentStyle: ViewStyle | null = active ? { justifyContent: "flex-start", paddingBottom: viewport } : null;

  // A scroll asked before the taller content is laid out is clamped to the old
  // height, so it is asked again once the content has its new size.
  return { scrollRef, onViewportLayout, onFieldLayout, onContentSizeChange: scrollToTarget, contentStyle };
}
