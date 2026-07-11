import { useCallback, useEffect, useRef, useState } from "react";
import { View, Text, FlatList, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { useLocalSearchParams, useRouter, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceGetLibrary, useLibraryServiceListMediaItems } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";
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

export default function LibraryDetailScreen() {
  const { libraryId } = useLocalSearchParams<{ libraryId: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [allItems, setAllItems] = useState<V1MediaItem[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const prevDataRef = useRef<unknown>(null);

  const { data: library } = useLibraryServiceGetLibrary(libraryId ?? "");

  const { data, isLoading, isError, isFetching, refetch } = useLibraryServiceListMediaItems(
    {
      libraryId,
      "pagination.page": page,
      "pagination.pageSize": PAGE_SIZE,
    },
    { query: { enabled: !!libraryId } },
  );

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

  const handleEndReached = useCallback(() => {
    if (!isFetching && hasMore) setPage((p) => p + 1);
  }, [isFetching, hasMore]);

  const renderItem = useCallback(
    ({ item, index }: { item: V1MediaItem; index: number }) => (
      <FocusableCard
        onPress={() => router.push(`/media/${item.id}`)}
        hasTVPreferredFocus={index === 0}
        style={styles.gridItem}
      >
        <View style={[styles.poster, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 18 }}>?</Text>
        </View>
        <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
          {item.title}
        </Text>
      </FocusableCard>
    ),
    [theme, router],
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
      ) : isLoading && page === 1 ? (
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
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 80 },
  gridItem: { width: CARD_WIDTH, alignItems: "center" },
  poster: {
    width: CARD_WIDTH,
    height: POSTER_HEIGHT,
    justifyContent: "center",
    alignItems: "center",
  },
  title: { fontSize: 16, fontWeight: "500", marginTop: 8, textAlign: "center" },
});
