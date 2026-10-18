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
  hasMediaItemCount,
} from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";
import type { V1Library, V1LibraryType } from "@evey/api";

const LIBRARY_TYPE_ICONS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "🎬",
  LIBRARY_TYPE_SHOWS: "📺",
  LIBRARY_TYPE_MUSIC: "🎵",
  LIBRARY_TYPE_PHOTOS: "📷",
};

const LIBRARY_TYPE_KEYS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "media.library.movies",
  LIBRARY_TYPE_SHOWS: "media.library.tvShows",
  LIBRARY_TYPE_MUSIC: "media.library.music",
  LIBRARY_TYPE_PHOTOS: "media.library.photos",
};

function getTypeIcon(type?: V1LibraryType): string {
  return (type && LIBRARY_TYPE_ICONS[type]) || "📁";
}

// The count arrives with the library itself (Library.item_count, computed by
// ListLibraries in one grouped query). The card used to send its own
// ListMediaItems request for it, one per library, to read the same total.
//
// Music and photo libraries hold no media_items (their content lives in the
// music/photo domains), so that count is 0 for them - which rendered a
// confident "0 items" under a Musique library holding 3 albums. Those types
// get no count rather than a wrong one; the dedicated screen shows the real
// figures. Guarded by hasMediaItemCount so it can never drift from the
// redirect table that decides which types are dedicated. An absent count (the
// server could not read it, or predates the field) shows nothing either.
function LibraryItemCount({ library }: { library: V1Library }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const total = library.itemCount;
  if (!hasMediaItemCount(library.type) || total === undefined) return null;
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
          <LibraryItemCount library={item} />
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

  // This screen used to state the failure and stop there - no retry anywhere on
  // it, so a single failed request left the whole library list unreachable
  // until the app was killed and relaunched.
  if (isError) {
    return <LoadFailed onRetry={() => void refetch()} />;
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
