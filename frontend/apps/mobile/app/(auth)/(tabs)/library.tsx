import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { useLibraryServiceListLibraries } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import type { V1Library, V1LibraryType } from "@evey/api";

const LIBRARY_TYPE_ICONS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "🎬",
  LIBRARY_TYPE_SHOWS: "📺",
  LIBRARY_TYPE_MUSIC: "🎵",
  LIBRARY_TYPE_PHOTOS: "📷",
  LIBRARY_TYPE_BOOKS: "📚",
};

function getTypeIcon(type?: V1LibraryType): string {
  return (type && LIBRARY_TYPE_ICONS[type]) || "📁";
}

export default function LibraryScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { data, isLoading, isError, refetch } =
    useLibraryServiceListLibraries();

  const libraries = data?.libraries ?? [];

  const renderItem = ({ item }: { item: V1Library }) => (
    <TouchableOpacity
      style={[
        styles.card,
        {
          backgroundColor: theme.colors.surface,
          borderRadius: theme.radii.md,
        },
      ]}
      activeOpacity={0.7}
      onPress={() => router.push(`/library/${item.id}`)}
      accessible={true}
      accessibilityLabel={item.name}
      accessibilityRole="button"
    >
      <Text style={styles.icon}>{getTypeIcon(item.type)}</Text>
      <View style={{ flex: 1 }}>
        <Text style={[styles.name, { color: theme.colors.text }]}>
          {item.name}
        </Text>
        <Text style={[styles.type, { color: theme.colors.textMuted }]}>
          {item.type?.replace("LIBRARY_TYPE_", "").toLowerCase()}
        </Text>
      </View>
    </TouchableOpacity>
  );

  if (isLoading) {
    return (
      <View
        style={[styles.center, { backgroundColor: theme.colors.bg }]}
      >
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (isError) {
    return (
      <View
        style={[styles.center, { backgroundColor: theme.colors.bg }]}
      >
        <Text style={{ color: theme.colors.textMuted }}>
          Failed to load libraries
        </Text>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text
        style={[
          styles.header,
          { color: theme.colors.text, marginTop: theme.spacing.xl },
        ]}
      >
        Libraries
      </Text>
      <FlatList
        data={libraries}
        keyExtractor={(item) => item.id ?? ""}
        renderItem={renderItem}
        contentContainerStyle={{ padding: theme.spacing.md }}
        ItemSeparatorComponent={() => (
          <View style={{ height: theme.spacing.sm }} />
        )}
        refreshing={false}
        onRefresh={refetch}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 28, fontWeight: "700", paddingHorizontal: 16 },
  card: {
    flexDirection: "row",
    alignItems: "center",
    padding: 16,
  },
  icon: { fontSize: 28, marginRight: 14 },
  name: { fontSize: 17, fontWeight: "600" },
  type: { fontSize: 13, marginTop: 2 },
});
