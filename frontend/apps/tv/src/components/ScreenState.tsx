import { View, Text, ActivityIndicator, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { router } from "expo-router";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { FONT_SIZE } from "@evey/theme";

// The three states a screen can be in before it has content, in one place.
//
// Every TV screen fetched data and handled exactly one of them: loading. A
// request that FAILED left `data` undefined with `isLoading` false, and the
// usual `if (isLoading || !data) return <spinner/>` then span forever. On a
// television that is indistinguishable from a frozen application - there is no
// pull-to-refresh, no address bar, and at three metres nobody is going to
// guess that BACK still works.
//
// So an error state is not decoration here, it is the difference between "the
// app is broken" and "the server did not answer, press OK to try again". The
// retry button also satisfies the house rule that no state may leave the
// remote without a focusable target.

interface ScreenStateProps {
  /** react-query's isLoading (or any "still waiting" flag). */
  loading?: boolean;
  /** react-query's isError, or simply "the data never arrived". */
  error?: boolean;
  /** Retry. Omit and the screen offers going back instead. */
  onRetry?: () => void;
  /** Shown instead of the generic wording when the failure is understood. */
  message?: string;
}

/**
 * Renders the current non-content state, or null when there is nothing to say
 * (the caller then renders its own content).
 */
export function ScreenState({ loading, error, onRetry, message }: ScreenStateProps) {
  const theme = useTheme();
  const { t } = useTranslation();

  if (loading) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (!error) return null;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.title, { color: theme.colors.text }]}>
        {message ?? t("common.somethingWentWrong", "Something went wrong")}
      </Text>
      <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
        {t("common.offline.description", "Check your internet connection and try again.")}
      </Text>
      <View style={styles.actions}>
        {/* Exactly one hasTVPreferredFocus per screen (house rule): retry when
            it is offered, going back otherwise. */}
        {onRetry ? (
          <FocusableCard
            onPress={onRetry}
            hasTVPreferredFocus
            accessibilityLabel={t("common.retry", "Retry")}
            style={[styles.button, { backgroundColor: theme.colors.accent }]}
          >
            <Text style={[styles.buttonLabel, { color: theme.colors.onAccent }]}>
              {t("common.retry", "Retry")}
            </Text>
          </FocusableCard>
        ) : null}
        <FocusableCard
          onPress={() => router.back()}
          hasTVPreferredFocus={!onRetry}
          accessibilityLabel={t("common.back", "Back")}
          style={[styles.button, { backgroundColor: theme.colors.surface }]}
        >
          <Text style={[styles.buttonLabel, { color: theme.colors.text }]}>
            {t("common.back", "Back")}
          </Text>
        </FocusableCard>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, justifyContent: "center", alignItems: "center", gap: 12, padding: 48 },
  title: { fontSize: FONT_SIZE.xl, fontWeight: "600", textAlign: "center" },
  hint: { fontSize: FONT_SIZE.base, textAlign: "center", maxWidth: 900 },
  actions: { flexDirection: "row", gap: 12, marginTop: 16 },
  button: { paddingVertical: 10, paddingHorizontal: 36, borderRadius: 8 },
  buttonLabel: { fontSize: FONT_SIZE.lg, fontWeight: "600" },
});
