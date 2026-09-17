import { View, Text, Pressable, ActivityIndicator, StatusBar, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

// usePlaySession's ShedReason type isn't exported (usePlaySession.ts is out of
// scope for this refactor) - mirrored here for the non-null variant this
// screen is rendered with.
type ShedReason = "too_many_streams" | "file_not_ready" | "calibrating" | "file_needs_remux";

interface ShedScreenProps {
  shedReason: ShedReason;
  onRetry: () => void;
  onBack: () => void;
}

export function ShedScreen({ shedReason, onRetry, onBack }: ShedScreenProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <StatusBar hidden />
      <Ionicons name="time-outline" size={ICON_SIZE["3xl"]} color={theme.colors.textMuted} />
      <Text style={{ color: theme.colors.text, fontSize: FONT_SIZE.lg, fontWeight: "600", marginTop: 16, textAlign: "center", paddingHorizontal: 24 }}>
        {t(`player.shed.${shedReason === "file_not_ready" ? "fileNotReady" : shedReason === "calibrating" ? "calibrating" : shedReason === "file_needs_remux" ? "fileNeedsRemux" : "tooManyStreams"}.title`)}
      </Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.sm, marginTop: 8, textAlign: "center", paddingHorizontal: 32 }}>
        {t(`player.shed.${shedReason === "file_not_ready" ? "fileNotReady" : shedReason === "calibrating" ? "calibrating" : shedReason === "file_needs_remux" ? "fileNeedsRemux" : "tooManyStreams"}.body`)}
      </Text>
      <Pressable
        onPress={onRetry}
        style={{ marginTop: 20 }}
      >
        <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.base, fontWeight: "600" }}>{t("player.retry")}</Text>
      </Pressable>
      <Pressable onPress={onBack} style={{ marginTop: 16 }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.sm }}>{t("player.goBack")}</Text>
      </Pressable>
    </View>
  );
}

interface ErrorScreenProps {
  onBack: () => void;
  // Optional: absent for a caller that has nothing sensible to retry (kept
  // optional rather than forcing every call site to pass a no-op).
  onRetry?: () => void;
}

export function ErrorScreen({ onBack, onRetry }: ErrorScreenProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <StatusBar hidden />
      <Text style={{ color: theme.colors.text, fontSize: FONT_SIZE.base }}>{t("player.failedToLoadStream")}</Text>
      {onRetry && (
        <Pressable onPress={onRetry} style={{ marginTop: 16 }}>
          <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.base, fontWeight: "600" }}>{t("player.retry")}</Text>
        </Pressable>
      )}
      <Pressable onPress={onBack} style={{ marginTop: 16 }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.sm }}>{t("player.goBack")}</Text>
      </Pressable>
    </View>
  );
}

export function LoadingScreen() {
  const theme = useTheme();
  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <StatusBar hidden />
      <ActivityIndicator size="large" color={theme.colors.accent} />
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
});
