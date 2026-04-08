import { useEffect, useRef } from "react";
import {
  View,
  Text,
  Pressable,
  Animated,
  StyleSheet,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useToastStore, type Toast, type ToastType } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";

const ICON_MAP: Record<ToastType, keyof typeof Ionicons.glyphMap> = {
  success: "checkmark-circle",
  error: "alert-circle",
  info: "information-circle",
  warning: "warning",
};

function ToastItem({ toast }: { toast: Toast }) {
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
      <Ionicons name={ICON_MAP[toast.type]} size={20} color={color} style={styles.icon} />
      <Text style={[styles.message, { color: theme.colors.text }]} numberOfLines={2}>
        {toast.message}
      </Text>
      <Pressable onPress={() => remove(toast.id)} hitSlop={8}>
        <Ionicons name="close" size={18} color={theme.colors.textMuted} />
      </Pressable>
    </Animated.View>
  );
}

export function ToastContainer() {
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
  message: { flex: 1, fontSize: 14 },
});
