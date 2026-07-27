import React, { Component } from "react";
import { View, Text, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import type { ThemeTokens } from "@evey/theme";
import { FONT_SIZE } from "@evey/theme";

interface Props {
  children?: React.ReactNode;
  theme: ThemeTokens;
  t: TFunction;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

class ErrorBoundaryInner extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  handleRetry = () => {
    this.setState({ hasError: false, error: null });
  };

  handleGoHome = () => {
    this.setState({ hasError: false, error: null });
    router.replace("/(auth)/(tabs)");
  };

  render() {
    if (this.state.hasError) {
      const { theme, t } = this.props;
      return (
        <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
          <Text style={[styles.icon, { color: theme.colors.error }]}>!</Text>
          <Text style={[styles.title, { color: theme.colors.text }]}>{t("common.somethingWentWrong")}</Text>
          <Text style={[styles.message, { color: theme.colors.textMuted }]} numberOfLines={3}>
            {this.state.error?.message ?? t("common.error")}
          </Text>
          <View style={styles.actions}>
            {/* Only one hasTVPreferredFocus on this screen (see agents.md rule) -
                the retry action is the more common recovery path, so it gets
                initial focus. Go Home stays reachable via the d-pad. */}
            <FocusableCard
              onPress={this.handleRetry}
              hasTVPreferredFocus={true}
              accessibilityLabel={t("common.tryAgain")}
              style={[styles.actionBtn, { backgroundColor: theme.colors.accent }]}
            >
              <Text style={[styles.retryText, { color: theme.colors.onAccent }]}>{t("common.tryAgain")}</Text>
            </FocusableCard>
            <FocusableCard
              onPress={this.handleGoHome}
              accessibilityLabel={t("common.nav.home")}
              style={[styles.actionBtn, { backgroundColor: theme.colors.surface }]}
            >
              <Text style={[styles.retryText, { color: theme.colors.text }]}>{t("common.nav.home")}</Text>
            </FocusableCard>
          </View>
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
  actionBtn: { paddingVertical: 14, paddingHorizontal: 36, borderRadius: 8 },
  retryText: { fontWeight: "600", fontSize: FONT_SIZE.xl },
});
