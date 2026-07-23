import { useCallback, useEffect, useRef, useState } from "react";
import { View, Text, FlatList, ScrollView, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { useLocalSearchParams, useRouter, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetLibrary,
  useLibraryServiceListMediaItems,
  useLibraryServiceListGenres,
  libraryServiceListMediaItems,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { artworkUrl } from "../../../src/lib/artwork";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { CARD_WIDTH_GRID, POSTER_RATIO, LABEL_BLOCK_HEIGHT, GRID_GAP } from "../../../src/lib/tvLayout";

const NUM_COLUMNS = 5;
const CARD_WIDTH = CARD_WIDTH_GRID;
const POSTER_HEIGHT = CARD_WIDTH * POSTER_RATIO;
const ROW_HEIGHT = POSTER_HEIGHT + LABEL_BLOCK_HEIGHT + GRID_GAP;
const PAGE_SIZE = 30;

function tvLibKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

const tvLibGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * Math.floor(index / NUM_COLUMNS),
  index,
});

// D-pad genre filter: a row of focusable chips (same pattern as the search
// screen's ScopeTab) rather than a dropdown, which has no natural TV/remote
// affordance.
function GenreChip({ label, active, onPress }: { label: string; active: boolean; onPress: () => void }) {
  const theme = useTheme();
  return (
    <FocusableCard
      onPress={onPress}
      style={[
        styles.genreChip,
        { backgroundColor: active ? theme.colors.accent : theme.colors.surface },
      ]}
      accessibilityLabel={label}
      accessibilityState={{ selected: active }}
    >
      <Text style={{ color: active ? "#fff" : theme.colors.text, fontSize: 18, fontWeight: "600" }}>{label}</Text>
    </FocusableCard>
  );
}

export default function LibraryDetailScreen() {
  const { libraryId } = useLocalSearchParams<{ libraryId: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [page, setPage] = useState(1);
  const [allItems, setAllItems] = useState<V1MediaItem[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const [genre, setGenre] = useState("");
  const [shuffling, setShuffling] = useState(false);
  // Ref latch, not just the state flag: a second remote OK press before the
  // setShuffling(true) re-render would read the old state and slip a second
  // probe + navigation through. The ref updates synchronously, closing that
  // window (FocusableCard has no disabled prop to fall back on).
  const shufflingRef = useRef(false);
  const prevDataRef = useRef<unknown>(null);

  const { data: library } = useLibraryServiceGetLibrary(libraryId ?? "");
  const { data: genresData } = useLibraryServiceListGenres(libraryId ?? "", {
    query: { enabled: !!libraryId },
  });
  const genres = genresData?.genres ?? [];

  const { data, isLoading, isError, isFetching, refetch } = useLibraryServiceListMediaItems(
    {
      libraryId,
      ...(genre ? { genre } : {}),
      "pagination.page": page,
      "pagination.pageSize": PAGE_SIZE,
    },
    { query: { enabled: !!libraryId } },
  );

  // Reset pagination when the genre filter changes. Declared BEFORE the
  // accumulate effect ON PURPOSE: React runs effects in declaration order, so
  // on the render where genre flips to a value whose page-1 is already cached
  // (query resolves synchronously), this clear must run FIRST and the
  // accumulate effect repopulates SECOND. The reverse order let the accumulate
  // fill allItems and this clear wipe it to [] with no re-run (its [data,page]
  // deps were unchanged) - a persistent empty grid on the common peek-a-genre-
  // then-back-to-All flow over a static library.
  useEffect(() => {
    setPage(1);
    setAllItems([]);
    setHasMore(true);
    prevDataRef.current = null;
  }, [genre]);

  useEffect(() => {
    if (!data || data === prevDataRef.current) return;
    prevDataRef.current = data;
    const newItems = data.items ?? [];
    if (page === 1) {
      setAllItems(newItems);
    } else {
      setAllItems((prev) => [...prev, ...newItems]);
    }
    const totalPages = data.pagination?.totalPages ?? 1;
    setHasMore(page < totalPages);
  }, [data, page]);

  // Shuffle play: no local-only facet exists on this screen (the genre
  // filter is server-side), so a shuffle always draws from the WHOLE library
  // via a random page offset (pageSize 1) rather than the loaded slice -
  // the genre filter, if any, still applies server-side since it's baked
  // into the probe request.
  const handleShuffle = useCallback(async () => {
    if (shufflingRef.current || !libraryId) return;
    shufflingRef.current = true;
    setShuffling(true);
    try {
      const base = { libraryId, ...(genre ? { genre } : {}) };
      const probe = await libraryServiceListMediaItems({ ...base, "pagination.page": 1, "pagination.pageSize": 1 });
      const total = probe.pagination?.totalItems ?? 0;
      if (total === 0) { toast.info(t("common.shuffleEmpty")); return; }
      const randomPage = Math.floor(Math.random() * total) + 1;
      const res = randomPage === 1
        ? probe
        : await libraryServiceListMediaItems({ ...base, "pagination.page": randomPage, "pagination.pageSize": 1 });
      const pick = res.items?.[0];
      if (pick?.id) router.push(`/player/${pick.id}`);
      else toast.error(t("common.shuffleFailed"));
    } catch {
      toast.error(t("common.shuffleFailed"));
    } finally {
      shufflingRef.current = false;
      setShuffling(false);
    }
  }, [libraryId, genre, router, t]);

  const handleEndReached = useCallback(() => {
    if (!isFetching && hasMore) setPage((p) => p + 1);
  }, [isFetching, hasMore]);

  const renderItem = useCallback(
    ({ item, index }: { item: V1MediaItem; index: number }) => {
      const posterUri = item.id ? artworkUrl(serverUrl, item.id, "poster", CARD_WIDTH * 2) : undefined;
      const blurhash = (item.artwork ?? []).find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.blurhash;
      return (
        <FocusableCard
          onPress={() => router.push(`/media/${item.id}`)}
          hasTVPreferredFocus={index === 0}
          style={styles.gridItem}
        >
          <View style={[styles.poster, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface, overflow: "hidden" }]}>
            {posterUri ? (
              <Image
                source={{ uri: posterUri, headers: authHeaders }}
                style={StyleSheet.absoluteFill}
                contentFit="cover"
                transition={200}
                recyclingKey={item.id}
                placeholder={blurhash ? { blurhash } : undefined}
                placeholderContentFit="cover"
              />
            ) : (
              <Text style={{ color: theme.colors.textMuted, fontSize: 18 }}>?</Text>
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

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: library?.name ?? "Library",
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      {isError ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 12 }}>{t("common.loadError")}</Text>
          <Pressable onPress={() => refetch()}>
            <Text style={{ color: theme.colors.accent, fontSize: 20 }}>{t("common.retry")}</Text>
          </Pressable>
        </View>
      ) : (
        <>
          <View style={styles.toolbar}>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ flex: 1 }} contentContainerStyle={{ gap: 12 }}>
              <GenreChip label={t("media.search.allGenres") || "All"} active={!genre} onPress={() => setGenre("")} />
              {genres.map((g) => (
                <GenreChip key={g} label={g} active={genre === g} onPress={() => setGenre(genre === g ? "" : g)} />
              ))}
            </ScrollView>
            <FocusableCard
              onPress={handleShuffle}
              style={[styles.shuffleBtn, { backgroundColor: theme.colors.surface }]}
              accessibilityLabel={t("common.shufflePlay")}
            >
              {shuffling ? (
                <ActivityIndicator color={theme.colors.accent} size="small" />
              ) : (
                <Ionicons name="shuffle" size={24} color={theme.colors.text} />
              )}
              <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "600" }}>{t("common.shufflePlay")}</Text>
            </FocusableCard>
          </View>
          {isLoading && page === 1 ? (
            <View style={styles.center}>
              <ActivityIndicator color={theme.colors.accent} size="large" />
            </View>
          ) : (
            <FlatList
              data={allItems}
              keyExtractor={tvLibKeyExtractor}
              renderItem={renderItem}
              numColumns={NUM_COLUMNS}
              contentContainerStyle={{ padding: 24 }}
              columnWrapperStyle={{ gap: 16, marginBottom: 16 }}
              onEndReached={handleEndReached}
              onEndReachedThreshold={0.5}
              getItemLayout={tvLibGetItemLayout}
              initialNumToRender={10}
              maxToRenderPerBatch={15}
              windowSize={5}
              removeClippedSubviews={true}
              ListFooterComponent={
                isFetching && page > 1 ? (
                  <ActivityIndicator color={theme.colors.accent} style={{ marginVertical: 24 }} />
                ) : null
              }
              ListEmptyComponent={
                <View style={styles.center}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("common.noItems")}</Text>
                </View>
              }
            />
          )}
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 80 },
  toolbar: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 24,
    paddingTop: 16,
    gap: 16,
  },
  genreChip: {
    paddingHorizontal: 20,
    paddingVertical: 10,
  },
  shuffleBtn: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    paddingHorizontal: 20,
    paddingVertical: 10,
  },
  gridItem: { width: CARD_WIDTH, alignItems: "center" },
  poster: {
    width: CARD_WIDTH,
    height: POSTER_HEIGHT,
    justifyContent: "center",
    alignItems: "center",
  },
  title: { fontSize: 16, fontWeight: "500", marginTop: 8, textAlign: "center" },
});
