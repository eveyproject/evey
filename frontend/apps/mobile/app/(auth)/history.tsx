import { useState, useCallback, useEffect, useMemo } from "react";
import { View, Text, SectionList, Pressable, Alert, RefreshControl, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { router, Stack } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  usePlaybackServiceGetWatchHistory,
  usePlaybackServiceMarkAsUnwatched,
  usePlaybackServiceClearWatchHistory,
  getPlaybackServiceGetWatchHistoryQueryKey,
} from "@evey/api";
import type { V1WatchProgress } from "@evey/api";
import { toast } from "@evey/store";
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

const DAY_MS = 24 * 60 * 60 * 1000;

// Mirrors the web WatchHistory grouping so both clients bucket the same way.
function getDateGroupLabel(iso: string | undefined, t: TFunction): string {
  if (!iso) return t("common.unknown", "Unknown");
  const date = new Date(iso);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const itemDate = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const diffDays = Math.floor((today.getTime() - itemDate.getTime()) / DAY_MS);

  if (diffDays === 0) return t("auth.watchHistory.today", "Today");
  if (diffDays === 1) return t("auth.watchHistory.yesterday", "Yesterday");
  if (diffDays < 7) return t("auth.watchHistory.thisWeek", "This Week");
  if (diffDays < 30) return t("auth.watchHistory.thisMonth", "This Month");
  return date.toLocaleDateString(undefined, { year: "numeric", month: "long" });
}

// Server now returns title + (for episodes) series title + season/episode
// numbers, so we show a real label instead of the bare media id.
function getItemLabel(item: V1WatchProgress, t: TFunction): string {
  const title = item.title?.trim();
  if (item.mediaType === "episode") {
    const s = item.parentIndexNumber;
    const e = item.indexNumber;
    const code = s && e ? `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}` : e ? `E${e}` : "";
    const parts = [item.seriesTitle?.trim(), code, title].filter(Boolean);
    if (parts.length) return parts.join(" · ");
  }
  return title || t("auth.watchHistory.unknownMedia", "Unknown media");
}

function HistoryItem({ item, onRemove, isPending }: {
  item: V1WatchProgress;
  onRemove: () => void;
  isPending: boolean;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const imageUri = item.mediaItemId
    ? `${serverUrl}/api/v1/media/${item.mediaItemId}/artwork/poster?w=200`
    : undefined;
  const imageSource = useAuthenticatedImage(imageUri);
  const pct = item.percentComplete ?? 0;
  const label = getItemLabel(item, t);

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
      accessibilityLabel={label}
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
          {label}
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

  const sections = useMemo(() => {
    const grouped = items.reduce<Record<string, V1WatchProgress[]>>((acc, item) => {
      const group = getDateGroupLabel(item.lastPlayedAt, t);
      (acc[group] ??= []).push(item);
      return acc;
    }, {});
    return Object.entries(grouped).map(([title, data]) => ({ title, data }));
  }, [items, t]);

  const unwatch = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
      },
      onError: () => toast.error(t("media.history.removeFailed", "Failed to remove from history")),
    },
  });

  const clearAll = usePlaybackServiceClearWatchHistory({
    mutation: {
      onSuccess: () => {
        setPage(1);
        setHasMore(true);
        void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
        toast.success(t("auth.watchHistory.cleared", "History cleared"));
      },
      onError: () => toast.error(t("auth.watchHistory.clearFailed", "Failed to clear history")),
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
          onPress: () => clearAll.mutate({}),
        },
      ],
    );
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("auth.watchHistory.title", "Watch History"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => items.length > 0 ? (
          <Pressable onPress={handleClear} accessible accessibilityLabel={t("auth.watchHistory.clearHistory")} accessibilityRole="button">
            <Ionicons name="trash-outline" size={22} color={theme.colors.textMuted} />
          </Pressable>
        ) : undefined,
      }} />

      {!isLoading && items.length === 0 ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: theme.spacing.xl }}>
          <Ionicons name="time-outline" size={48} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: theme.spacing.md, textAlign: "center" }}>
            {t("auth.watchHistory.noHistory", "No watch history")}
          </Text>
        </View>
      ) : (
        <SectionList
          sections={sections}
          keyExtractor={historyKeyExtractor}
          renderItem={renderHistoryItem}
          renderSectionHeader={({ section }) => (
            <View style={{
              paddingHorizontal: theme.spacing.md,
              paddingTop: theme.spacing.sm,
              paddingBottom: theme.spacing.xs,
              backgroundColor: theme.colors.bg,
            }}>
              <Text style={{
                color: theme.colors.textMuted,
                fontSize: 12,
                fontWeight: "600",
                textTransform: "uppercase",
                letterSpacing: 0.5,
              }}>
                {section.title}
              </Text>
            </View>
          )}
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
          stickySectionHeadersEnabled={false}
        />
      )}
    </View>
  );
}
