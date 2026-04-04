import { Component, type ReactNode } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <View style={styles.container}>
          <Ionicons name="alert-circle-outline" size={48} color="#ef4444" />
          <Text style={styles.title}>Something went wrong</Text>
          <Text style={styles.message} numberOfLines={3}>
            {this.state.error?.message ?? "Unknown error"}
          </Text>
          <Pressable
            onPress={() => this.setState({ hasError: false, error: null })}
            style={styles.retryBtn}
          >
            <Text style={styles.retryText}>Try Again</Text>
          </Pressable>
        </View>
      );
    }

    return this.props.children;
  }
}

const styles = StyleSheet.create({
  container: { flex: 1, justifyContent: "center", alignItems: "center", padding: 24, backgroundColor: "#0c1220" },
  title: { color: "#fff", fontSize: 18, fontWeight: "700", marginTop: 16 },
  message: { color: "rgba(255,255,255,0.5)", fontSize: 13, textAlign: "center", marginTop: 8, maxWidth: 280 },
  retryBtn: { marginTop: 20, backgroundColor: "#818cf8", borderRadius: 8, paddingVertical: 10, paddingHorizontal: 24 },
  retryText: { color: "#fff", fontWeight: "600", fontSize: 15 },
});
