import { useState } from "react";
import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueries } from "@tanstack/react-query";
import { useLibraryServiceListLibraries, libraryServiceListGenres, hasMediaItemCount } from "@evey/api";
import type { V1Library } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { mergeGenres, genreTranslationKey, genreHref } from "../../../src/lib/genres";
import { DEEP_SCREEN_GUTTER } from "../../../src/lib/tvLayout";

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

// Every tile used to be filled with the accent colour, and the focus ring is
// drawn in that same colour: on the television the whole grid was red and
// nothing said where the remote was (2026-09-19). Focus now CHANGES the tile
// rather than outlining it - the one you are on is the only filled one.
function GenreTile({ label, isFirst, onPress }: { label: string; isFirst: boolean; onPress: () => void }) {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  return (
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      style={[
        styles.tile,
        {
          backgroundColor: focused ? theme.colors.accent : theme.colors.surface,
          borderRadius: theme.radii.md,
        },
      ]}
      accessibilityLabel={label}
    >
      <Text
        style={[styles.tileText, { color: focused ? theme.colors.onAccent : theme.colors.text }]}
        numberOfLines={2}
      >
        {label}
      </Text>
    </FocusableCard>
  );
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

  // A ListGenres call that failed hands mergeGenres an undefined list, which it
  // cannot tell apart from a library that genuinely has no genres. Without this
  // the picker just came up short: every genre of the failing library vanished,
  // with nothing on screen to say a request had failed and no way to retry.
  //
  // One failure blanks the whole picker rather than showing a partial list.
  // That is the deliberate choice - a picker that silently omits a third of the
  // library is the bug being fixed here, and a partial list with a warning
  // would need UI this screen does not have.
  const genresError = genreQueries.some((q) => q.isError);

  if (librariesError || genresError) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 20, marginBottom: 16 }}>{t("common.loadError")}</Text>
        <FocusableCard
          onPress={() => {
            void refetch();
            genreQueries.forEach((q) => void q.refetch());
          }}
          hasTVPreferredFocus
          accessibilityLabel={t("common.retry")}
        >
          <Text style={{ color: theme.colors.accent, fontSize: 18 }}>{t("common.retry")}</Text>
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
        columnWrapperStyle={genres.length > 0 ? { gap: 12, marginBottom: 16 } : undefined}
        renderItem={({ item: genre, index }) => (
          <GenreTile
            label={t(genreTranslationKey(genre), genre)}
            isFirst={index === 0}
            onPress={() => router.push(genreHref(genre))}
          />
        )}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 20, marginBottom: 16 }}>{t("common.noGenres")}</Text>
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
              <Text style={{ color: theme.colors.accent, fontSize: 17, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
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
  header: { fontSize: 30, fontWeight: "700", paddingHorizontal: DEEP_SCREEN_GUTTER, paddingTop: 32, paddingBottom: 16 },
  tile: { flex: 1, paddingVertical: 32, paddingHorizontal: 24, alignItems: "center", justifyContent: "center", minHeight: 120 },
  tileText: { fontSize: 21, fontWeight: "700", textAlign: "center" },
  emptyAction: { paddingHorizontal: 24, paddingVertical: 10 },
});
