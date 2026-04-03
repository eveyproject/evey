import { View, Text, Pressable, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { router } from "expo-router";
import { usePlayerStore } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";
import { useServerUrl } from "../hooks/useServerUrl";

export function MiniPlayer() {
  const theme = useTheme();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();

  const mediaId = usePlayerStore((s) => s.mediaId);
  const mediaTitle = usePlayerStore((s) => s.mediaTitle);
  const isActive = usePlayerStore((s) => s.isActive);
  const mode = usePlayerStore((s) => s.mode);
  const stop = usePlayerStore((s) => s.stop);

  if (!isActive || mode !== "mini" || !mediaId) return null;

  const artworkUri = serverUrl ? `${serverUrl}/api/v1/media/${mediaId}/artwork/poster` : undefined;

  return (
    <Pressable
      style={[styles.container, { backgroundColor: theme.colors.surfaceRaised, borderTopColor: theme.colors.border }]}
      onPress={() => router.push(`/(auth)/player/${mediaId}`)}
    >
      {artworkUri ? (
        <Image
          source={{ uri: artworkUri, headers: authHeaders }}
          style={[styles.artwork, { borderRadius: theme.radii.sm }]}
          contentFit="cover"
        />
      ) : (
        <View style={[styles.artwork, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }]}>
          <Ionicons name="musical-notes" size={18} color={theme.colors.textMuted} />
        </View>
      )}
      <View style={styles.info}>
        <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>
          {mediaTitle ?? "Now Playing"}
        </Text>
      </View>
      <Pressable onPress={stop} hitSlop={12} style={styles.closeBtn}>
        <Ionicons name="close" size={20} color={theme.colors.textMuted} />
      </Pressable>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  container: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 12,
    paddingVertical: 8,
    borderTopWidth: StyleSheet.hairlineWidth,
  },
  artwork: { width: 40, height: 40, marginRight: 10 },
  info: { flex: 1 },
  title: { fontSize: 14, fontWeight: "600" },
  closeBtn: { padding: 8 },
});
