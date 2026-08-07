import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";

// "The server did not answer", said once.
//
// Screens that dropped isError rendered their EMPTY state instead - "no
// tracks", "no photos" - which tells the user their library is empty when it
// is merely unreachable. That is not a missing nicety, it is the app being
// confidently wrong, and it sends people looking for a scanning problem that
// does not exist.
//
// Every screen goes through here. Favorites and Library used to carry their
// own inline copy, and Library's had no retry at all - a single failed request
// left the whole list unreachable short of restarting the app.

interface LoadFailedProps {
  /** Retry. Omit only when there is genuinely nothing to re-run. */
  onRetry?: () => void;
  /** Overrides the generic wording when the cause is understood. */
  message?: string;
  /** Sits inside a tab or a section rather than filling the screen. */
  inline?: boolean;
}

export function LoadFailed({ onRetry, message, inline }: LoadFailedProps) {
  const theme = useTheme();
  const { t } = useTranslation();

  return (
    <View style={[inline ? styles.inline : styles.full, { backgroundColor: inline ? "transparent" : theme.colors.bg }]}>
      <Text style={[styles.message, { color: theme.colors.textMuted }]}>
        {message ?? t("common.loadError")}
      </Text>
      {onRetry ? (
        <Pressable
          onPress={onRetry}
          accessible={true}
          accessibilityRole="button"
          accessibilityLabel={t("common.retry")}
          style={styles.retry}
        >
          <Text style={{ color: theme.colors.accent, fontWeight: "600" }}>{t("common.retry")}</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  full: { flex: 1, justifyContent: "center", alignItems: "center", padding: 24 },
  inline: { alignItems: "center", paddingVertical: 48, paddingHorizontal: 24 },
  message: { marginBottom: 12, textAlign: "center" },
  retry: { paddingVertical: 8, paddingHorizontal: 16 },
});
