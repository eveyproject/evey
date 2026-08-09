import React, { Component } from "react";
import { View, Text, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import type { ThemeTokens } from "@evey/theme";
import { FONT_SIZE } from "@evey/theme";
import { logger } from "../lib/log";
import { crashContext } from "../lib/crashGuard";

interface Props {
  children?: React.ReactNode;
  theme: ThemeTokens;
  t: TFunction;
}

interface State {
  hasError: boolean;
  error: Error | null;
  /** Retries so far, so a deterministic failure stops offering a useless button. */
  attempts: number;
}

// Past this, retrying is not a recovery path any more - the failure is
// deterministic and the button just replays it. The viewer is then offered the
// one action that does change something: leaving the screen.
const MAX_RETRIES = 2;

// Enough to see what led here, few enough to stay readable from a sofa.
const CONTEXT_LINES = 6;

class ErrorBoundaryInner extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null, attempts: 0 };
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { hasError: true, error };
  }

  // A contained error is still an error: without this line the app degrades
  // gracefully and nobody ever learns why.
  componentDidCatch(error: Error, info: { componentStack?: string | null }) {
    logger("ui").error(`écran en erreur: ${error.message}`, info.componentStack ?? "");
  }

  // Clearing hasError is enough to remount: React already unmounted the whole
  // subtree when it rendered the fallback, so the children come back fresh.
  // What `attempts` buys is the count below - a deterministic failure would
  // otherwise offer a button that replays it for ever.
  handleRetry = () => {
    this.setState((s) => ({ hasError: false, error: null, attempts: s.attempts + 1 }));
  };

  handleGoHome = () => {
    this.setState({ hasError: false, error: null, attempts: 0 });
    router.replace("/(auth)/(tabs)");
  };

  render() {
    if (this.state.hasError) {
      const { theme, t } = this.props;
      const canRetry = this.state.attempts < MAX_RETRIES;
      return (
        <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
          <Text style={[styles.icon, { color: theme.colors.error }]}>!</Text>
          <Text style={[styles.title, { color: theme.colors.text }]}>{t("common.somethingWentWrong")}</Text>
          <Text style={[styles.message, { color: theme.colors.textMuted }]} numberOfLines={3}>
            {this.state.error?.message ?? t("common.error")}
          </Text>
          <View style={styles.actions}>
            {/* Exactly one hasTVPreferredFocus (see agents.md rule): retry while
                it is still plausible, then Home - which is always offered, so
                the remote is never left without a target. */}
            {canRetry ? (
              <FocusableCard
                onPress={this.handleRetry}
                hasTVPreferredFocus={true}
                accessibilityLabel={t("common.tryAgain")}
                style={[styles.actionBtn, { backgroundColor: theme.colors.accent }]}
              >
                <Text style={[styles.retryText, { color: theme.colors.onAccent }]}>{t("common.tryAgain")}</Text>
              </FocusableCard>
            ) : null}
            <FocusableCard
              onPress={this.handleGoHome}
              hasTVPreferredFocus={!canRetry}
              accessibilityLabel={t("common.nav.home")}
              style={[styles.actionBtn, { backgroundColor: theme.colors.surface }]}
            >
              <Text style={[styles.retryText, { color: theme.colors.text }]}>{t("common.nav.home")}</Text>
            </FocusableCard>
          </View>
          {/* Only once retrying is off the table: the failure is deterministic
              and somebody will have to report it, from a room with no console. */}
          {!canRetry ? (
            <Text
              testID="crash-context"
              style={[styles.context, { color: theme.colors.textMuted }]}
              numberOfLines={CONTEXT_LINES}
            >
              {crashContext().split("\n").slice(-CONTEXT_LINES).join("\n")}
            </Text>
          ) : null}
        </View>
      );
    }

    return this.props.children;
  }
}

export function ErrorBoundary({ children }: { children: React.ReactNode }) {
  const theme = useTheme();
  const { t } = useTranslation();
  return React.createElement(ErrorBoundaryInner, { theme, t }, children);
}

const styles = StyleSheet.create({
  container: { flex: 1, justifyContent: "center", alignItems: "center", padding: 80 },
  icon: { fontSize: 64, fontWeight: "bold", marginBottom: 16 },
  title: { fontSize: FONT_SIZE["4xl"], fontWeight: "700", marginTop: 16 },
  message: { fontSize: FONT_SIZE.xl, textAlign: "center", marginTop: 12, maxWidth: 500 },
  actions: { flexDirection: "row", gap: 16, marginTop: 32 },
  context: { fontSize: FONT_SIZE.sm, textAlign: "left", marginTop: 32, maxWidth: 900 },
  actionBtn: { paddingVertical: 14, paddingHorizontal: 36, borderRadius: 8 },
  retryText: { fontWeight: "600", fontSize: FONT_SIZE.xl },
});
