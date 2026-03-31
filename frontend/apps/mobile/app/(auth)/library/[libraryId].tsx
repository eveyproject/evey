import { useCallback, useEffect, useRef, useState } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  ActivityIndicator,
  Dimensions,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import { Skeleton } from "../../../src/components/Skeleton";
import { useLocalSearchParams, useRouter, Stack } from "expo-router";
import {
  useLibraryServiceGetLibrary,
  useLibraryServiceListMediaItems,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";

const NUM_COLUMNS = 3;
const SPACING = 8;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH = (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;
const PAGE_SIZE = 30;

function getPoster(item: V1MediaItem): string | undefined {
  return item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url;
}

export default function LibraryDetailScreen() {
  const { libraryId } = useLocalSearchParams<{ libraryId: string }>();
  const theme = useTheme();
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [allItems, setAllItems] = useState<V1MediaItem[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const prevDataRef = useRef<unknown>(null);

  const authHeaders = useAuthHeaders();
  const { data: library } = useLibraryServiceGetLibrary(libraryId ?? "");

  const { data, isLoading, isFetching, refetch } =
    useLibraryServiceListMediaItems(
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

  const handleRefresh = useCallback(() => {
    setPage(1);
    setHasMore(true);
  }, []);

  const handleEndReached = useCallback(() => {
    if (!isFetching && hasMore) {
      setPage((p) => p + 1);
    }
  }, [isFetching, hasMore]);

  const renderItem = useCallback(
    ({ item }: { item: V1MediaItem }) => {
      const poster = getPoster(item);
      return (
        <TouchableOpacity
          style={[
            styles.gridItem,
            {
              width: ITEM_WIDTH,
              marginLeft: SPACING,
              marginBottom: SPACING,
            },
          ]}
          activeOpacity={0.7}
          onPress={() => router.push(`/media/${item.id}`)}
        >
          {poster ? (
            <Image
              source={{ uri: poster, headers: authHeaders }}
              style={[
                styles.poster,
                {
                  width: ITEM_WIDTH,
                  height: POSTER_HEIGHT,
                  borderRadius: theme.radii.sm,
                },
              ]}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View
              style={[
                styles.posterPlaceholder,
                {
                  width: ITEM_WIDTH,
                  height: POSTER_HEIGHT,
                  borderRadius: theme.radii.sm,
                  backgroundColor: theme.colors.surfaceRaised,
                },
              ]}
            >
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                No poster
              </Text>
            </View>
          )}
          <Text
            style={[styles.title, { color: theme.colors.text }]}
            numberOfLines={2}
          >
            {item.title}
          </Text>
        </TouchableOpacity>
      );
    },
    [theme, router, authHeaders],
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
      {isLoading && page === 1 ? (
        <View style={{ flexDirection: "row", flexWrap: "wrap", paddingTop: SPACING }}>
          {Array.from({ length: 9 }).map((_, i) => (
            <View key={i} style={{ marginLeft: SPACING, marginBottom: SPACING, alignItems: "center", width: ITEM_WIDTH }}>
              <Skeleton width={ITEM_WIDTH} height={POSTER_HEIGHT} borderRadius={theme.radii.sm} />
              <Skeleton width={ITEM_WIDTH * 0.7} height={12} borderRadius={4} style={{ marginTop: 6 }} />
            </View>
          ))}
        </View>
      ) : (
        <FlatList
          data={allItems}
          keyExtractor={(item) => item.id ?? ""}
          renderItem={renderItem}
          numColumns={NUM_COLUMNS}
          contentContainerStyle={{ paddingTop: SPACING }}
          refreshing={isFetching && page === 1}
          onRefresh={handleRefresh}
          onEndReached={handleEndReached}
          onEndReachedThreshold={0.5}
          ListFooterComponent={
            isFetching && page > 1 ? (
              <ActivityIndicator
                color={theme.colors.accent}
                style={{ marginVertical: 16 }}
              />
            ) : null
          }
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted }}>
                No items in this library
              </Text>
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
  gridItem: { alignItems: "center" },
  poster: { width: "100%" },
  posterPlaceholder: {
    justifyContent: "center",
    alignItems: "center",
  },
  title: {
    fontSize: 12,
    fontWeight: "500",
    marginTop: 4,
    textAlign: "center",
  },
});
