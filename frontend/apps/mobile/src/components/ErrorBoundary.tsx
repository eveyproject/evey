import React, { Component } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import type { ThemeTokens } from "@evey/theme";
import type { TFunction } from "i18next";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

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

  render() {
    if (this.state.hasError) {
      const { theme, t } = this.props;
      return (
        <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
          <Ionicons name="alert-circle-outline" size={ICON_SIZE["4xl"]} color={theme.colors.error} />
          <Text style={[styles.title, { color: theme.colors.text }]}>{t("common.somethingWentWrong", "Something went wrong")}</Text>
          <Text style={[styles.message, { color: theme.colors.textMuted }]} numberOfLines={3}>
            {this.state.error?.message ?? t("common.unknownError", "Unknown error")}
          </Text>
          <Pressable
            onPress={() => this.setState({ hasError: false, error: null })}
            style={[styles.retryBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
          >
            <Text style={[styles.retryText, { color: theme.colors.text }]}>{t("common.tryAgain", "Try Again")}</Text>
          </Pressable>
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
  container: { flex: 1, justifyContent: "center", alignItems: "center", padding: 24 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700", marginTop: 16 },
  message: { fontSize: FONT_SIZE.xs, textAlign: "center", marginTop: 8, maxWidth: 280 },
  retryBtn: { marginTop: 20, paddingVertical: 10, paddingHorizontal: 24 },
  retryText: { fontWeight: "600", fontSize: FONT_SIZE.sm },
});
