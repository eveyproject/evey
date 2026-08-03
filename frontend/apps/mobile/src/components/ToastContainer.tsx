import { memo, useEffect, useRef } from "react";
import {
  View,
  Text,
  Pressable,
  Animated,
  StyleSheet,
} from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
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
// re-renders.
const ToastItem = memo(function ToastItem({ toast }: { toast: Toast }) {
  const { t } = useTranslation();
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

    const duration = toast.duration ?? 3000;
    const timer = setTimeout(() => {
      Animated.timing(opacity, { toValue: 0, duration: 200, useNativeDriver: true }).start(() =>
        remove(toast.id),
      );
    }, duration);

    return () => clearTimeout(timer);
  }, []);

  const color = typeColors[toast.type];

  return (
    <Animated.View
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
      <Pressable
        style={styles.body}
        disabled={!toast.onPress}
        onPress={() => {
          toast.onPress?.();
          remove(toast.id);
        }}
        accessible
        accessibilityRole={toast.onPress ? "button" : undefined}
        accessibilityLabel={toast.message}
      >
        <Ionicons name={ICON_MAP[toast.type]} size={ICON_SIZE.lg} color={color} style={styles.icon} />
        <Text style={[styles.message, { color: theme.colors.text }]} numberOfLines={2}>
          {toast.message}
        </Text>
      </Pressable>
      <Pressable onPress={() => remove(toast.id)} hitSlop={8} accessible accessibilityLabel={t("common.aria.dismiss", "Dismiss")} accessibilityRole="button">
        <Ionicons name="close" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
      </Pressable>
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
  container: {
    position: "absolute",
    top: 50,
    left: 16,
    right: 16,
    zIndex: 9999,
    gap: 8,
  },
  toast: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 12,
    paddingHorizontal: 14,
    borderRadius: 10,
    borderLeftWidth: 4,
    elevation: 6,
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.3,
    shadowRadius: 4,
  },
  icon: { marginRight: 10 },
  body: { flex: 1, flexDirection: "row", alignItems: "center" },
  message: { flex: 1, fontSize: FONT_SIZE.sm },
});
