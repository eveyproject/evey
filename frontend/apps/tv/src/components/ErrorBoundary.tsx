import React, { Component } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTheme } from "../providers/ThemeProvider";
import type { ThemeTokens } from "@evey/theme";
import { FONT_SIZE } from "@evey/theme";

interface Props {
  children?: React.ReactNode;
  theme: ThemeTokens;
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
      const { theme } = this.props;
      return (
        <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
          <Text style={[styles.icon, { color: theme.colors.error }]}>!</Text>
          <Text style={[styles.title, { color: theme.colors.text }]}>Something went wrong</Text>
          <Text style={[styles.message, { color: theme.colors.textMuted }]} numberOfLines={3}>
            {this.state.error?.message ?? "Unknown error"}
          </Text>
          <Pressable
            onPress={() => this.setState({ hasError: false, error: null })}
            style={[styles.retryBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
            accessible={true}
            accessibilityLabel="Try Again"
            accessibilityRole="button"
          >
            <Text style={[styles.retryText, { color: theme.colors.text }]}>Try Again</Text>
          </Pressable>
        </View>
      );
    }

    return this.props.children;
  }
}

export function ErrorBoundary({ children }: { children: React.ReactNode }) {
  const theme = useTheme();
  return React.createElement(ErrorBoundaryInner, { theme }, children);
}

const styles = StyleSheet.create({
  container: { flex: 1, justifyContent: "center", alignItems: "center", padding: 80 },
  icon: { fontSize: 64, fontWeight: "bold", marginBottom: 16 },
  title: { fontSize: FONT_SIZE["4xl"], fontWeight: "700", marginTop: 16 },
  message: { fontSize: FONT_SIZE.xl, textAlign: "center", marginTop: 12, maxWidth: 500 },
  retryBtn: { marginTop: 32, paddingVertical: 14, paddingHorizontal: 36 },
  retryText: { fontWeight: "600", fontSize: FONT_SIZE.xl },
});
