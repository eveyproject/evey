import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useLibraryServiceListLibraries } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import type { V1Library, V1LibraryType } from "@evey/api";
import { FocusableCard } from "../../../src/components/FocusableCard";

const LIBRARY_TYPE_LABELS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "Movies",
  LIBRARY_TYPE_SHOWS: "Shows",
  LIBRARY_TYPE_MUSIC: "Music",
  LIBRARY_TYPE_PHOTOS: "Photos",
  LIBRARY_TYPE_BOOKS: "Books",
};

function getTypeLabel(type?: V1LibraryType): string {
  return (type && LIBRARY_TYPE_LABELS[type]) || "Library";
}

export default function LibraryScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { data, isLoading, isError } = useLibraryServiceListLibraries();

  const libraries = data?.libraries ?? [];

  const renderItem = ({ item, index }: { item: V1Library; index: number }) => (
    <FocusableCard
      onPress={() => router.push(`/library/${item.id}`)}
      hasTVPreferredFocus={index === 0}
      style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
    >
      <Text style={[styles.name, { color: theme.colors.text }]}>{item.name}</Text>
      <Text style={[styles.type, { color: theme.colors.textMuted }]}>
        {getTypeLabel(item.type)}
      </Text>
    </FocusableCard>
  );

  if (isLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (isError) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>Failed to load libraries</Text>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>Libraries</Text>

      <View style={styles.linkRow}>
        <FocusableCard
          onPress={() => router.push("/(auth)/collections")}
          style={[styles.linkCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        >
          <Text style={[styles.linkText, { color: theme.colors.accent }]}>Collections</Text>
        </FocusableCard>
        <FocusableCard
          onPress={() => router.push("/(auth)/playlists")}
          style={[styles.linkCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        >
          <Text style={[styles.linkText, { color: theme.colors.accent }]}>Playlists</Text>
        </FocusableCard>
      </View>

      <FlatList
        data={libraries}
        keyExtractor={(item) => item.id ?? ""}
        renderItem={renderItem}
        contentContainerStyle={{ padding: 24 }}
        ItemSeparatorComponent={() => <View style={{ height: 16 }} />}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32 },
  card: {
    padding: 24,
  },
  name: { fontSize: 24, fontWeight: "600" },
  type: { fontSize: 18, marginTop: 4 },
  linkRow: { flexDirection: "row", gap: 16, paddingHorizontal: 24, paddingTop: 16 },
  linkCard: { paddingVertical: 16, paddingHorizontal: 32 },
  linkText: { fontSize: 22, fontWeight: "600" },
});
