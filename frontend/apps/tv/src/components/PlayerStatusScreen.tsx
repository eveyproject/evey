import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";

// What the player shows instead of a picture: a server not ready yet, an
// expired session, a stream that failed. One action that can help, then a
// way back; the remote lands on the action.
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
      {icon ? <Ionicons name={icon} size={48} color="rgba(255,255,255,0.7)" /> : null}
      <Text style={[styles.title, icon ? styles.titleUnderIcon : null]}>{title}</Text>
      {body ? <Text style={styles.body}>{body}</Text> : null}
      <Pressable onPress={onAction} style={{ marginTop: body ? 28 : 24 }} hasTVPreferredFocus>
        <Text style={[styles.action, { color: theme.colors.accent }]}>{actionLabel}</Text>
      </Pressable>
      <Pressable onPress={onBack} style={{ marginTop: 20 }}>
        <Text style={styles.back}>{t("player.goBack", "Go back")}</Text>
      </Pressable>
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
  container: { flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: "#000" },
  title: { color: "#fff", fontSize: 21, textAlign: "center", paddingHorizontal: 60 },
  titleUnderIcon: { fontWeight: "600", marginTop: 20 },
  body: { color: "rgba(255,255,255,0.7)", fontSize: 17, marginTop: 12, textAlign: "center", paddingHorizontal: 80 },
  action: { fontSize: 20, fontWeight: "600" },
  back: { color: "rgba(255,255,255,0.7)", fontSize: 17 },
});
