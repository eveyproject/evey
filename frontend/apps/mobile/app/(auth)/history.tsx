import { useState, useCallback, useEffect } from "react";
import { View, Text, FlatList, Pressable, Alert, RefreshControl, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  usePlaybackServiceGetWatchHistory,
  usePlaybackServiceMarkAsUnwatched,
  getPlaybackServiceGetWatchHistoryQueryKey,
} from "@evey/api";
import type { V1WatchProgress } from "@evey/api";
import { useTheme } from "../../src/providers/ThemeProvider";
import { useServerUrl } from "../../src/hooks/useServerUrl";
import { useAuthenticatedImage } from "../../src/hooks/useAuthenticatedImage";
import { ProgressBar } from "../../src/components/ProgressBar";

function formatDuration(ms: string | undefined): string {
  if (!ms) return "";
  const totalMin = Math.floor(Number(ms) / 60000);
  if (totalMin < 60) return `${totalMin}min`;
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return m > 0 ? `${h}h ${m}min` : `${h}h`;
}

function formatDate(iso: string | undefined): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function HistoryItem({ item, onRemove, isPending }: {
  item: V1WatchProgress;
  onRemove: () => void;
  isPending: boolean;
}) {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const imageUri = item.mediaItemId
    ? `${serverUrl}/api/v1/media/${item.mediaItemId}/artwork/poster`
    : undefined;
  const imageSource = useAuthenticatedImage(imageUri);
  const pct = item.percentComplete ?? 0;

  return (
    <Pressable
      onPress={() => item.mediaItemId && router.push(`/media/${item.mediaItemId}`)}
      style={{
        flexDirection: "row",
        alignItems: "center",
        padding: theme.spacing.md,
        gap: theme.spacing.sm,
        borderBottomWidth: StyleSheet.hairlineWidth,
        borderBottomColor: theme.colors.border,
        backgroundColor: theme.colors.surface,
      }}
      accessible
      accessibilityLabel={item.mediaItemId ?? "History item"}
      accessibilityRole="button"
    >
      <View style={{
        width: 48,
        height: 68,
        borderRadius: theme.radii.sm,
        backgroundColor: theme.colors.surfaceRaised,
        overflow: "hidden",
      }}>
        {imageSource && (
          <Image source={imageSource} style={{ width: 48, height: 68 }} contentFit="cover" />
        )}
      </View>

      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "500" }} numberOfLines={1}>
          {item.mediaItemId?.slice(0, 8)}
          {item.playCount && item.playCount > 1 ? ` (x${item.playCount})` : ""}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {formatDate(item.lastPlayedAt)}
        </Text>
        {item.durationMs && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
            {formatDuration(item.positionMs)} / {formatDuration(item.durationMs)}
          </Text>
        )}
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6, marginTop: 2 }}>
          <View style={{ width: 80 }}>
            <ProgressBar percent={pct} />
          </View>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{Math.round(pct)}%</Text>
        </View>
      </View>

      <Pressable
        onPress={onRemove}
        disabled={isPending}
        hitSlop={8}
        accessible
        accessibilityLabel="Remove"
        accessibilityRole="button"
        style={{ opacity: isPending ? 0.5 : 1 }}
      >
        <Ionicons name="close-circle-outline" size={22} color={theme.colors.textMuted} />
      </Pressable>
    </Pressable>
  );
}

function historyKeyExtractor(item: V1WatchProgress) {
  return item.id ?? item.mediaItemId ?? "";
}

const PAGE_SIZE = 30;

export default function HistoryScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);
  const [page, setPage] = useState(1);
  const [allItems, setAllItems] = useState<V1WatchProgress[]>([]);
  const [hasMore, setHasMore] = useState(true);

  const { data, isLoading, isFetching, refetch } = usePlaybackServiceGetWatchHistory({ "pagination.page": page, "pagination.pageSize": PAGE_SIZE });

  useEffect(() => {
    if (!data?.items) return;
    const newItems = data.items;
    if (page === 1) {
      setAllItems(newItems);
    } else {
      setAllItems((prev) => [...prev, ...newItems]);
    }
    setHasMore(newItems.length >= PAGE_SIZE);
  }, [data, page]);

  const items = allItems;

  const unwatch = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
      },
    },
  });

  const renderHistoryItem = useCallback(
    ({ item }: { item: V1WatchProgress }) => (
      <HistoryItem
        item={item}
        onRemove={() => item.mediaItemId && unwatch.mutate({ mediaItemId: item.mediaItemId })}
        isPending={unwatch.isPending}
      />
    ),
    [unwatch],
  );

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    setPage(1);
    setHasMore(true);
    await refetch();
    setRefreshing(false);
  }, [refetch]);

  const handleEndReached = useCallback(() => {
    if (!isFetching && hasMore) {
      setPage((p) => p + 1);
    }
  }, [isFetching, hasMore]);

  function handleClear() {
    Alert.alert(
      t("auth.watchHistory.clearTitle"),
      t("auth.watchHistory.clearConfirm"),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("auth.watchHistory.clearAll"),
          style: "destructive",
          onPress: () => {
            void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
          },
        },
      ],
    );
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <View style={{
        flexDirection: "row",
        alignItems: "center",
        justifyContent: "space-between",
        paddingHorizontal: theme.spacing.md,
        paddingTop: theme.spacing.lg,
        paddingBottom: theme.spacing.sm,
      }}>
        <Pressable onPress={() => router.back()} accessible accessibilityLabel="Back" accessibilityRole="button">
          <Ionicons name="chevron-back" size={24} color={theme.colors.text} />
        </Pressable>
        <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "600" }}>
          {t("auth.watchHistory.title", "Watch History")}
        </Text>
        {items.length > 0 ? (
          <Pressable onPress={handleClear} accessible accessibilityLabel={t("auth.watchHistory.clearHistory")} accessibilityRole="button">
            <Ionicons name="trash-outline" size={22} color={theme.colors.textMuted} />
          </Pressable>
        ) : (
          <View style={{ width: 22 }} />
        )}
      </View>

      {!isLoading && items.length === 0 ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: theme.spacing.xl }}>
          <Ionicons name="time-outline" size={48} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: theme.spacing.md, textAlign: "center" }}>
            {t("auth.watchHistory.noHistory", "No watch history")}
          </Text>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={historyKeyExtractor}
          renderItem={renderHistoryItem}
          refreshControl={
            <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />
          }
          contentContainerStyle={{ paddingBottom: theme.spacing.xl }}
          onEndReached={handleEndReached}
          onEndReachedThreshold={0.5}
          initialNumToRender={10}
          maxToRenderPerBatch={10}
          windowSize={5}
          removeClippedSubviews={true}
        />
      )}
    </View>
  );
}
