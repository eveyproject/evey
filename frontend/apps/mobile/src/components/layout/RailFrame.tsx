import { useRef, useState, useCallback } from "react";
import { View, LayoutAnimation, Platform, UIManager, type GestureResponderEvent } from "react-native";
import { useTheme } from "../../providers/ThemeProvider";
import { NavRail } from "./NavRail";
import { createRailIntentTracker } from "./railIntent";

if (Platform.OS === "android" && UIManager.setLayoutAnimationEnabledExperimental) {
  UIManager.setLayoutAnimationEnabledExperimental(true);
}

// Content frame with the auto-hiding nav rail: the rail slides away when the
// user dives into content and slides back when they scroll up - visible when
// navigating is likely, gone while consuming. Touches are observed here (they
// bubble to ancestors without claiming the responder), so the individual
// screens need no wiring.
export function RailFrame({ mode, children }: { mode: "main" | "admin"; children: React.ReactNode }) {
  const theme = useTheme();
  const [railVisible, setRailVisible] = useState(true);
  const tracker = useRef(createRailIntentTracker()).current;

  const setVisible = useCallback((next: boolean) => {
    setRailVisible((prev) => {
      if (prev === next) return prev;
      LayoutAnimation.configureNext?.(LayoutAnimation.Presets.easeInEaseOut);
      return next;
    });
  }, []);

  const point = (e: GestureResponderEvent) => {
    const n = e.nativeEvent as { pageX?: number; pageY?: number; touches?: Array<{ pageX: number; pageY: number }> };
    const t = n.touches?.[0];
    return { x: t?.pageX ?? n.pageX ?? 0, y: t?.pageY ?? n.pageY ?? 0 };
  };

  const onTouchStart = useCallback((e: GestureResponderEvent) => {
    const { x, y } = point(e);
    tracker.start(y, x);
  }, [tracker]);

  const onTouchMove = useCallback((e: GestureResponderEvent) => {
    const { x, y } = point(e);
    const intent = tracker.move(y, x);
    if (intent) setVisible(intent === "show");
  }, [tracker, setVisible]);

  return (
    <View style={{ flex: 1, flexDirection: "row", backgroundColor: theme.colors.surface }}>
      {railVisible && <NavRail mode={mode} />}
      <View style={{ flex: 1 }} onTouchStart={onTouchStart} onTouchMove={onTouchMove}>
        {children}
      </View>
    </View>
  );
}
