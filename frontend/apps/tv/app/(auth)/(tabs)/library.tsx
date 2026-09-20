import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceListLibraries } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import type { V1Library, V1LibraryType } from "@evey/api";
import { FocusableCard } from "../../../src/components/FocusableCard";

// Mirrors the mobile/web LIBRARY_TYPE_KEYS mapping (apps/mobile/app/(auth)/(tabs)/library.tsx,
// apps/web/src/routes/_auth/library/index.lazy.tsx): reuse the same i18n keys rather than
// duplicating hardcoded English labels.
const LIBRARY_TYPE_KEYS: Record<string, string> = {
  LIBRARY_TYPE_MOVIES: "media.library.movies",
  LIBRARY_TYPE_SHOWS: "media.library.tvShows",
  LIBRARY_TYPE_MUSIC: "media.library.music",
  LIBRARY_TYPE_PHOTOS: "media.library.photos",
};

function getTypeLabelKey(type?: V1LibraryType): string {
  return (type && LIBRARY_TYPE_KEYS[type]) || "media.library.title";
}

export default function LibraryScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const { data, isLoading, isError, refetch } = useLibraryServiceListLibraries();

  const libraries = data?.libraries ?? [];

  const renderItem = ({ item, index }: { item: V1Library; index: number }) => (
    <FocusableCard
      onPress={() => router.push(`/library/${item.id}`)}
      hasTVPreferredFocus={index === 0}
      style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      accessibilityLabel={item.name}
    >
      <Text style={[styles.name, { color: theme.colors.text }]}>{item.name}</Text>
      <Text style={[styles.type, { color: theme.colors.textMuted }]}>
        {t(getTypeLabelKey(item.type))}
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
        <Text style={{ color: theme.colors.textMuted, fontSize: 20, marginBottom: 16 }}>
          {t("common.loadError")}
        </Text>
        <FocusableCard
          onPress={() => refetch()}
          hasTVPreferredFocus
          style={[styles.linkCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
          accessibilityLabel={t("common.retry")}
        >
          <Text style={[styles.linkText, { color: theme.colors.accent }]}>{t("common.retry")}</Text>
        </FocusableCard>
      </View>
    );
  }

  // When there are no libraries yet, the FlatList below renders nothing and
  // never claims hasTVPreferredFocus (that only fires on its first item) -
  // without an explicit fallback here, this screen left the remote with no
  // initial focus target at all. The two link cards are the only focusable
  // elements left in that case.
  const noLibraries = libraries.length === 0;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("common.nav.libraries")}</Text>

      <View style={styles.linkRow}>
        <FocusableCard
          onPress={() => router.push("/(auth)/collections")}
          hasTVPreferredFocus={noLibraries}
          style={[styles.linkCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
          accessibilityLabel={t("common.collections")}
        >
          <Text style={[styles.linkText, { color: theme.colors.accent }]}>{t("common.collections")}</Text>
        </FocusableCard>
        <FocusableCard
          onPress={() => router.push("/(auth)/playlists")}
          style={[styles.linkCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
          accessibilityLabel={t("common.playlists")}
        >
          <Text style={[styles.linkText, { color: theme.colors.accent }]}>{t("common.playlists")}</Text>
        </FocusableCard>
        {/* Genres sits here rather than in the sidebar: the genre screens live
            outside the (tabs) group, so a sidebar entry pointing at them would
            make the sidebar itself disappear on arrival. Collections and
            Playlists are reached the same way for the same reason. */}
        <FocusableCard
          onPress={() => router.push("/(auth)/genres")}
          style={[styles.linkCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
          accessibilityLabel={t("common.nav.genres")}
        >
          <Text style={[styles.linkText, { color: theme.colors.accent }]}>{t("common.nav.genres")}</Text>
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
  header: { fontSize: 30, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32 },
  card: {
    padding: 24,
  },
  name: { fontSize: 21, fontWeight: "600" },
  type: { fontSize: 17, marginTop: 4 },
  linkRow: { flexDirection: "row", gap: 12, paddingHorizontal: 24, paddingTop: 16 },
  linkCard: { paddingVertical: 11, paddingHorizontal: 32 },
  linkText: { fontSize: 20, fontWeight: "600" },
});
