import { View, Text, Pressable, ActivityIndicator, StatusBar, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";

// usePlaySession's ShedReason type isn't exported (usePlaySession.ts is out of
// scope for this refactor) - mirrored here for the non-null variant this
// screen is rendered with.
type ShedReason = "too_many_streams" | "file_not_ready";

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
      <Ionicons name="time-outline" size={40} color={theme.colors.textMuted} />
      <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "600", marginTop: 16, textAlign: "center", paddingHorizontal: 24 }}>
        {t(shedReason === "file_not_ready" ? "player.shed.fileNotReady.title" : "player.shed.tooManyStreams.title")}
      </Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 14, marginTop: 8, textAlign: "center", paddingHorizontal: 32 }}>
        {t(shedReason === "file_not_ready" ? "player.shed.fileNotReady.body" : "player.shed.tooManyStreams.body")}
      </Text>
      <Pressable
        onPress={onRetry}
        style={{ marginTop: 20 }}
      >
        <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>{t("player.retry")}</Text>
      </Pressable>
      <Pressable onPress={onBack} style={{ marginTop: 16 }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 14 }}>{t("player.goBack")}</Text>
      </Pressable>
    </View>
  );
}

interface ErrorScreenProps {
  onBack: () => void;
}

export function ErrorScreen({ onBack }: ErrorScreenProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <StatusBar hidden />
      <Text style={{ color: theme.colors.text, fontSize: 16 }}>{t("player.failedToLoadStream")}</Text>
      <Pressable onPress={onBack} style={{ marginTop: 16 }}>
        <Text style={{ color: theme.colors.accent, fontSize: 16 }}>{t("player.goBack")}</Text>
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
