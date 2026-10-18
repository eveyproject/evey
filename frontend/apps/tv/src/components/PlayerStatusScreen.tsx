import { View, Text, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";

// What the player shows instead of a picture: a server not ready yet, an
// expired session, a stream that failed. One action that can help, then a
// way back; the remote lands on the action. Same buttons as ScreenState, the
// rest of the app's error screens, on the player's black.
export function PlayerStatusScreen({
  icon,
  title,
  body,
  actionLabel,
  onAction,
  onBack,
}: {
  icon?: React.ComponentProps<typeof Ionicons>["name"];
  title: string;
  body?: string;
  actionLabel: string;
  onAction: () => void;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  const theme = useTheme();
  return (
    <View style={styles.container}>
      {icon ? <Ionicons name={icon} size={56} color="rgba(255,255,255,0.7)" /> : null}
      <Text style={styles.title}>{title}</Text>
      {body ? <Text style={styles.body}>{body}</Text> : null}
      <View style={styles.actions}>
        <FocusableCard
          onPress={onAction}
          hasTVPreferredFocus
          accessibilityLabel={actionLabel}
          style={[styles.button, { backgroundColor: theme.colors.accent }]}
        >
          <Text style={[styles.buttonLabel, { color: theme.colors.onAccent }]}>{actionLabel}</Text>
        </FocusableCard>
        <FocusableCard
          onPress={onBack}
          accessibilityLabel={t("player.goBack", "Go back")}
          style={[styles.button, { backgroundColor: theme.colors.surface }]}
        >
          <Text style={[styles.buttonLabel, { color: theme.colors.text }]}>{t("player.goBack", "Go back")}</Text>
        </FocusableCard>
      </View>
    </View>
  );
}

export function PlayerLoadingScreen() {
  const theme = useTheme();
  return (
    <View style={styles.container}>
      <ActivityIndicator size="large" color={theme.colors.accent} />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, justifyContent: "center", alignItems: "center", gap: 12, padding: 48, backgroundColor: "#000" },
  title: { color: "#fff", fontSize: FONT_SIZE["2xl"], fontWeight: "700", textAlign: "center", marginTop: 8 },
  body: { color: "rgba(255,255,255,0.7)", fontSize: FONT_SIZE.lg, textAlign: "center", maxWidth: 900 },
  actions: { flexDirection: "row", gap: 12, marginTop: 20 },
  button: { paddingVertical: 10, paddingHorizontal: 36, borderRadius: 8 },
  buttonLabel: { fontSize: FONT_SIZE.lg, fontWeight: "600" },
});
