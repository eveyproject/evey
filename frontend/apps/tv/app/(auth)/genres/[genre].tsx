import { useCallback } from "react";
import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, IN_LIBRARY_SEARCH_LIMIT } from "@evey/api";
import type { V1MediaItem, LibraryServiceSearchMediaSortField, LibraryServiceSearchMediaSortOrder } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { artworkUrl } from "../../../src/lib/artwork";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { genreTranslationKey } from "../../../src/lib/genres";
import { CARD_WIDTH_GRID, POSTER_RATIO, LABEL_BLOCK_HEIGHT, GRID_GAP } from "../../../src/lib/tvLayout";

const NUM_COLUMNS = 5;
const CARD_WIDTH = CARD_WIDTH_GRID;
const POSTER_HEIGHT = CARD_WIDTH * POSTER_RATIO;
const ROW_HEIGHT = POSTER_HEIGHT + LABEL_BLOCK_HEIGHT + GRID_GAP;

function genreMediaKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

const genreGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * Math.floor(index / NUM_COLUMNS),
  index,
});

export default function GenreDetailScreen() {
  const { genre } = useLocalSearchParams<{ genre: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  // Global (no library_id) SearchMedia with an empty query and only a genre
  // filter - this is the "flâner par genre" entry point across the whole
  // server, not one library's filter chip. Same fixed-size, non-paginated
  // page as the library screen's own SearchMedia usage (see
  // library/[libraryId].tsx: "search returns a single fixed-size page, no
  // cursor to page through") - IN_LIBRARY_SEARCH_LIMIT is generous enough for
  // a discovery screen.
  const { data, isLoading, isError, refetch } = useLibraryServiceSearchMedia(
    {
      genre: genre ?? "",
      limit: IN_LIBRARY_SEARCH_LIMIT,
      sortField: "MEDIA_SORT_FIELD_TITLE" as LibraryServiceSearchMediaSortField,
      sortOrder: "SORT_ORDER_ASC" as LibraryServiceSearchMediaSortOrder,
    },
    { query: { enabled: !!genre } },
  );

  const items = data?.items ?? [];
  const genreLabel = genre ? t(genreTranslationKey(genre), genre) : "";

  const renderItem = useCallback(
    ({ item, index }: { item: V1MediaItem; index: number }) => {
      const posterUri = item.id ? artworkUrl(serverUrl, item.id, "poster", CARD_WIDTH * 2) : undefined;
      const blurhash = (item.artwork ?? []).find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.blurhash;
      const rating = item.communityRating ?? 0;
      return (
        <FocusableCard
          onPress={() => router.push(`/media/${item.id}`)}
          hasTVPreferredFocus={index === 0}
          style={styles.gridItem}
          accessibilityLabel={item.title}
        >
          <View style={[styles.poster, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface, overflow: "hidden" }]}>
            {posterUri ? (
              <Image
                cachePolicy="memory-disk"
                source={{ uri: posterUri, headers: authHeaders }}
                style={StyleSheet.absoluteFill}
                contentFit="cover"
                transition={200}
                recyclingKey={item.id}
                placeholder={blurhash ? { blurhash } : undefined}
                placeholderContentFit="cover"
              />
            ) : (
              <Text style={{ color: theme.colors.textMuted, fontSize: 17 }}>?</Text>
            )}
            {rating > 0 && (
              <View
                style={[styles.ratingBadge, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                pointerEvents="none"
              >
                <Ionicons name="star" size={11} color={theme.colors.warning} />
                <Text style={[styles.ratingText, { color: theme.colors.text }]}>{rating.toFixed(1)}</Text>
              </View>
            )}
          </View>
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
            {item.title}
          </Text>
        </FocusableCard>
      );
    },
    [theme, router, serverUrl, authHeaders],
  );

  if (isError) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 20, marginBottom: 12 }}>{t("common.loadError")}</Text>
        <FocusableCard onPress={() => refetch()} hasTVPreferredFocus accessibilityLabel={t("common.retry")}>
          <Text style={{ color: theme.colors.accent, fontSize: 18 }}>{t("common.retry")}</Text>
        </FocusableCard>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{genreLabel}</Text>
      {isLoading ? (
        <View style={styles.center}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={genreMediaKeyExtractor}
          renderItem={renderItem}
          numColumns={NUM_COLUMNS}
          contentContainerStyle={{ padding: 24 }}
          columnWrapperStyle={items.length > 0 ? { gap: 12, marginBottom: 16 } : undefined}
          getItemLayout={genreGetItemLayout}
          initialNumToRender={10}
          maxToRenderPerBatch={15}
          windowSize={5}
          removeClippedSubviews={true}
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 20, marginBottom: 16 }}>{t("media.library.noItemsFiltered")}</Text>
              {/* Focusable target for the empty state - without it the remote
                  has nowhere to go and gets stuck on this screen. */}
              <FocusableCard
                onPress={() => router.push("/(auth)/genres")}
                hasTVPreferredFocus
                style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
                accessibilityLabel={t("common.nav.genres")}
              >
                <Text style={{ color: theme.colors.accent, fontSize: 17, fontWeight: "600" }}>{t("common.nav.genres")}</Text>
              </FocusableCard>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 30, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 16 },
  gridItem: { width: CARD_WIDTH, alignItems: "center" },
  poster: {
    width: CARD_WIDTH,
    height: POSTER_HEIGHT,
    justifyContent: "center",
    alignItems: "center",
  },
  title: { fontSize: 15, fontWeight: "500", marginTop: 8, textAlign: "center" },
  ratingBadge: {
    position: "absolute",
    bottom: 6,
    right: 6,
    flexDirection: "row",
    alignItems: "center",
    gap: 3,
    paddingHorizontal: 5,
    paddingVertical: 2,
  },
  ratingText: { fontSize: 11, fontWeight: "700" },
  emptyAction: { paddingHorizontal: 24, paddingVertical: 10 },
});
