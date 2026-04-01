import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
} from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import type { V1Library, V1LibraryType } from "@evey/api";

const LIBRARY_TYPE_ICONS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "🎬",
  LIBRARY_TYPE_SHOWS: "📺",
  LIBRARY_TYPE_MUSIC: "🎵",
  LIBRARY_TYPE_PHOTOS: "📷",
  LIBRARY_TYPE_BOOKS: "📚",
};

const LIBRARY_TYPE_KEYS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "media.library.movies",
  LIBRARY_TYPE_SHOWS: "media.library.tvShows",
  LIBRARY_TYPE_MUSIC: "media.library.music",
  LIBRARY_TYPE_PHOTOS: "media.library.photos",
  LIBRARY_TYPE_BOOKS: "media.library.books",
};

function getTypeIcon(type?: V1LibraryType): string {
  return (type && LIBRARY_TYPE_ICONS[type]) || "📁";
}

function LibraryItemCount({ libraryId }: { libraryId: string }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { data } = useLibraryServiceListMediaItems({
    libraryId,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });
  const total = data?.pagination?.totalItems;
  if (total === undefined) return null;
  return (
    <Text style={[styles.itemCount, { color: theme.colors.textMuted }]}>
      {t("common.item", { count: total })}
    </Text>
  );
}

export default function LibraryScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const { data, isLoading, isError, refetch } =
    useLibraryServiceListLibraries();

  const libraries = data?.libraries ?? [];

  const renderItem = ({ item }: { item: V1Library }) => {
    const typeKey =
      LIBRARY_TYPE_KEYS[item.type ?? ""] ?? "media.library.title";
    const label = t(typeKey);

    return (
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
          <View style={styles.nameRow}>
            <Text
              style={[styles.name, { color: theme.colors.text }]}
              numberOfLines={1}
            >
              {item.name}
            </Text>
            <View
              style={[
                styles.badge,
                { backgroundColor: theme.colors.surfaceRaised },
              ]}
            >
              <Text style={[styles.badgeText, { color: theme.colors.textMuted }]}>
                {label}
              </Text>
            </View>
          </View>
          {item.id && <LibraryItemCount libraryId={item.id} />}
          {item.updatedAt && (
            <Text style={[styles.updated, { color: theme.colors.textMuted }]}>
              {t("media.updated")}{" "}
              {new Date(item.updatedAt).toLocaleDateString(undefined, {
                month: "short",
                day: "numeric",
                year: "numeric",
              })}
            </Text>
          )}
        </View>
      </TouchableOpacity>
    );
  };

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
        <Text style={{ color: theme.colors.textMuted }}>
          {t("common.loadError")}
        </Text>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <View
        style={[
          styles.headerRow,
          { paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.xl },
        ]}
      >
        <Text style={[styles.header, { color: theme.colors.text }]}>
          {t("media.library.libraries")}
        </Text>
        {libraries.length > 0 && (
          <Text style={[styles.countLabel, { color: theme.colors.textMuted }]}>
            {libraries.length === 1
              ? `1 ${t("media.library.title").toLowerCase()}`
              : `${libraries.length} ${t("media.library.libraries").toLowerCase()}`}
          </Text>
        )}
      </View>

      {/* Quick links */}
      <View
        style={{
          flexDirection: "row",
          paddingHorizontal: theme.spacing.md,
          gap: theme.spacing.sm,
          marginTop: theme.spacing.md,
          marginBottom: theme.spacing.sm,
        }}
      >
        <TouchableOpacity
          style={[
            styles.quickLink,
            {
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
            },
          ]}
          activeOpacity={0.7}
          onPress={() => router.push("/collections")}
        >
          <Text style={styles.quickLinkIcon}>📂</Text>
          <Text style={[styles.quickLinkText, { color: theme.colors.text }]}>
            {t("common.collections")}
          </Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[
            styles.quickLink,
            {
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
            },
          ]}
          activeOpacity={0.7}
          onPress={() => router.push("/playlists")}
        >
          <Text style={styles.quickLinkIcon}>🎵</Text>
          <Text style={[styles.quickLinkText, { color: theme.colors.text }]}>
            {t("common.playlists")}
          </Text>
        </TouchableOpacity>
      </View>

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
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted }}>
              {t("media.library.noLibraries")}
            </Text>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  headerRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "baseline",
  },
  header: { fontSize: 28, fontWeight: "700" },
  countLabel: { fontSize: 13 },
  card: {
    flexDirection: "row",
    alignItems: "flex-start",
    padding: 16,
  },
  icon: { fontSize: 28, marginRight: 14, marginTop: 2 },
  nameRow: { flexDirection: "row", alignItems: "center", gap: 8 },
  name: { fontSize: 17, fontWeight: "600", flexShrink: 1 },
  badge: { paddingHorizontal: 8, paddingVertical: 2, borderRadius: 6 },
  badgeText: { fontSize: 11, fontWeight: "500" },
  itemCount: { fontSize: 13, marginTop: 2 },
  updated: { fontSize: 11, marginTop: 4 },
  quickLink: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    padding: 14,
  },
  quickLinkIcon: { fontSize: 20, marginRight: 10 },
  quickLinkText: { fontSize: 15, fontWeight: "600" },
});
