import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueries } from "@tanstack/react-query";
import { useLibraryServiceListLibraries, libraryServiceListGenres, hasMediaItemCount } from "@evey/api";
import type { V1Library } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { mergeGenres, genreTranslationKey, genreHref } from "../../../src/lib/genres";

const NUM_COLUMNS = 3;

// A music/photos library holds no media_items and so no ListMediaItems
// genres either (hasMediaItemCount, shared with the library screen's
// dedicated-route check) - skip it here rather than firing a ListGenres call
// that can only ever come back empty.
function isBrowsable(lib: V1Library): boolean {
  return hasMediaItemCount(lib.type);
}

function genreKeyExtractor(genre: string) {
  return genre;
}

export default function GenresScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();

  const { data: librariesData, isLoading: librariesLoading, isError: librariesError, refetch } = useLibraryServiceListLibraries();
  const libraryIds = (librariesData?.libraries ?? [])
    .filter(isBrowsable)
    .map((lib) => lib.id ?? "")
    .filter(Boolean);

  // One ListGenres call per browsable library, merged into a single global
  // picker: the library count is only known once ListLibraries resolves, so
  // this can't be a fixed number of useLibraryServiceListGenres calls -
  // useQueries is the standard react-query answer to a dynamic list of
  // independent queries.
  const genreQueries = useQueries({
    queries: libraryIds.map((id) => ({
      queryKey: ["tv-genres-for-picker", id],
      queryFn: () => libraryServiceListGenres(id),
      enabled: !!id,
    })),
  });

  const genresLoading = librariesLoading || genreQueries.some((q) => q.isLoading);
  const genres = mergeGenres(genreQueries.map((q) => q.data?.genres));

  if (librariesError) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 16 }}>{t("common.loadError")}</Text>
        <FocusableCard onPress={() => refetch()} hasTVPreferredFocus accessibilityLabel={t("common.retry")}>
          <Text style={{ color: theme.colors.accent, fontSize: 20 }}>{t("common.retry")}</Text>
        </FocusableCard>
      </View>
    );
  }

  if (genresLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("common.nav.genres")}</Text>
      <FlatList
        data={genres}
        keyExtractor={genreKeyExtractor}
        numColumns={NUM_COLUMNS}
        contentContainerStyle={{ padding: 24 }}
        columnWrapperStyle={genres.length > 0 ? { gap: 16, marginBottom: 16 } : undefined}
        renderItem={({ item: genre, index }) => (
          <FocusableCard
            onPress={() => router.push(genreHref(genre))}
            hasTVPreferredFocus={index === 0}
            style={[styles.tile, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
            accessibilityLabel={t(genreTranslationKey(genre), genre)}
          >
            <Text style={[styles.tileText, { color: theme.colors.onAccent }]} numberOfLines={2}>
              {t(genreTranslationKey(genre), genre)}
            </Text>
          </FocusableCard>
        )}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 16 }}>{t("common.noGenres")}</Text>
            {/* Focusable target for the empty state - without it the remote has
                nowhere to go and gets stuck on this screen (no other control
                is rendered when the list is empty). Mirrors history.tsx and
                collections/index.tsx. */}
            <FocusableCard
              onPress={() => router.push("/(auth)/(tabs)/library")}
              hasTVPreferredFocus
              style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
              accessibilityLabel={t("media.library.libraries")}
            >
              <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
            </FocusableCard>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 16 },
  tile: { flex: 1, paddingVertical: 32, paddingHorizontal: 24, alignItems: "center", justifyContent: "center", minHeight: 120 },
  tileText: { fontSize: 24, fontWeight: "700", textAlign: "center" },
  emptyAction: { paddingHorizontal: 24, paddingVertical: 14 },
});
