import { memo, useEffect, useRef } from "react";
import { View, Text, Animated, StyleSheet, AccessibilityInfo } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useToastStore, type Toast, type ToastType } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

const ICON_MAP: Record<ToastType, keyof typeof Ionicons.glyphMap> = {
  success: "checkmark-circle",
  error: "alert-circle",
  info: "information-circle",
  warning: "warning",
};

// memo: each toast is keyed and stable; re-mounts only when the toast
// array splices. Animations live in refs so per-frame ticks don't trigger
// re-renders. No dismiss control: a TV remote has no pointer, and
// auto-dismiss (toast.duration ?? 3000) is the only interaction a 10-foot
// UI needs here.
const ToastItem = memo(function ToastItem({ toast }: { toast: Toast }) {
  const theme = useTheme();
  const remove = useToastStore((s) => s.removeToast);
  const opacity = useRef(new Animated.Value(0)).current;
  const translateY = useRef(new Animated.Value(-20)).current;

  const typeColors: Record<ToastType, string> = {
    success: theme.colors.success,
    error: theme.colors.error,
    info: theme.colors.info,
    warning: theme.colors.warning,
  };

  useEffect(() => {
    Animated.parallel([
      Animated.timing(opacity, { toValue: 1, duration: 250, useNativeDriver: true }),
      Animated.timing(translateY, { toValue: 0, duration: 250, useNativeDriver: true }),
    ]).start();

    // Runs once, on mount - a toast is a fresh mount per id (memo above), so
    // this fires exactly once per toast appearance, never per re-render.
    AccessibilityInfo.announceForAccessibility(toast.message);

    const duration = toast.duration ?? 3000;
    const timer = setTimeout(() => {
      Animated.timing(opacity, { toValue: 0, duration: 200, useNativeDriver: true }).start(() =>
        remove(toast.id),
      );
    }, duration);

    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount-once: one entrance, announce and dismiss timer per toast
  }, []);

  const color = typeColors[toast.type];

  return (
    <Animated.View
      accessible
      accessibilityRole="alert"
      accessibilityLiveRegion="polite"
      style={[
        styles.toast,
        {
          backgroundColor: theme.colors.surfaceRaised,
          borderLeftColor: color,
          shadowColor: theme.colors.shadow,
          opacity,
          transform: [{ translateY }],
        },
      ]}
    >
      <Ionicons name={ICON_MAP[toast.type]} size={ICON_SIZE.xl} color={color} style={styles.icon} />
      <Text style={[styles.message, { color: theme.colors.text }]} numberOfLines={2}>
        {toast.message}
      </Text>
    </Animated.View>
  );
});

function ToastContainerImpl() {
  const toasts = useToastStore((s) => s.toasts);
  const visible = toasts.slice(-3);

  if (visible.length === 0) return null;

  return (
    <View style={styles.container} pointerEvents="box-none">
      {visible.map((t) => (
        <ToastItem key={t.id} toast={t} />
      ))}
    </View>
  );
}

// memo: no props; reacts to toast store via selector. Mounted in the
// root layout - keeps the toast overlay free from every parent re-render.
export const ToastContainer = memo(ToastContainerImpl);

const styles = StyleSheet.create({
  // Top-center, well clear of the screen edge (TV overscan) and centered
  // rather than edge-to-edge: a 10-foot UI is read from a couch, not held.
  container: {
    position: "absolute",
    top: 60,
    left: 0,
    right: 0,
    alignItems: "center",
    zIndex: 9999,
    gap: 10,
  },
  toast: {
    flexDirection: "row",
    alignItems: "center",
    minWidth: 420,
    maxWidth: 640,
    paddingVertical: 11,
    paddingHorizontal: 24,
    borderRadius: 12,
    borderLeftWidth: 6,
    elevation: 6,
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.3,
    shadowRadius: 4,
  },
  icon: { marginRight: 14 },
  message: { flex: 1, fontSize: FONT_SIZE.lg },
});
