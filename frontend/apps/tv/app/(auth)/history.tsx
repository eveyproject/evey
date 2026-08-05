import { useCallback } from "react";
import { View, Text, FlatList, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  usePlaybackServiceGetWatchHistory,
  usePlaybackServiceMarkAsUnwatched,
  getPlaybackServiceGetWatchHistoryQueryKey,
  watchHistoryLabel,
} from "@evey/api";
import type { V1WatchProgress } from "@evey/api";
import { formatDurationMs } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../src/providers/ThemeProvider";
import { progressPercent } from "../../src/lib/progressPercent";
import { ScreenState } from "../../src/components/ScreenState";
import { FocusableCard } from "../../src/components/FocusableCard";

function formatDate(iso: string | undefined): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function tvHistoryKeyExtractor(item: V1WatchProgress) {
  return item.id ?? item.mediaItemId ?? "";
}

function TVHistorySeparator() {
  return <View style={{ height: 4 }} />;
}

export default function HistoryScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, isError, refetch } = usePlaybackServiceGetWatchHistory({ limit: 50 });
  const items = data?.items ?? [];

  const unwatch = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
      },
      onError: () => toast.error(t("auth.watchHistory.clearFailed", "Could not remove from history")),
    },
  });

  const renderHistoryItem = useCallback(
    ({ item, index }: { item: V1WatchProgress; index: number }) => (
      <HistoryItem
        item={item}
        theme={theme}
        isFirst={index === 0}
        onPress={() => item.mediaItemId && router.push(`/media/${item.mediaItemId}`)}
        onRemove={() => item.mediaItemId && unwatch.mutate({ mediaItemId: item.mediaItemId })}
        isPending={unwatch.isPending}
      />
    ),
    [theme, unwatch, router],
  );

  // "Empty" and "the server did not answer" are different sentences, and this
  // screen used to say the first for both.
  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("common.watchHistory.title")}</Text>

      {items.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 16 }}>{t("auth.watchHistory.noHistory")}</Text>
          {/* Focusable target for the empty state - without it the remote has
              nowhere to go and gets stuck on this screen (no other control
              is rendered when the list is empty). */}
          <FocusableCard
            onPress={() => router.push("/(auth)/(tabs)/library")}
            hasTVPreferredFocus
            style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
            accessibilityLabel={t("media.library.libraries")}
          >
            <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
          </FocusableCard>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={tvHistoryKeyExtractor}
          contentContainerStyle={{ paddingHorizontal: 24, paddingBottom: 64 }}
          ItemSeparatorComponent={TVHistorySeparator}
          renderItem={renderHistoryItem}
          initialNumToRender={8}
          maxToRenderPerBatch={10}
          windowSize={5}
          removeClippedSubviews={true}
        />
      )}
    </View>
  );
}

function HistoryItem({
  item,
  theme,
  isFirst,
  onPress,
  onRemove,
  isPending,
}: {
  item: V1WatchProgress;
  theme: ReturnType<typeof useTheme>;
  isFirst?: boolean;
  onPress: () => void;
  onRemove: () => void;
  isPending: boolean;
}) {
  const { t } = useTranslation();
  const pct = progressPercent(item.percentComplete);
  const label = watchHistoryLabel(item) ?? t("auth.watchHistory.unknownMedia", "Unknown media");

  return (
    <View style={styles.rowWrap}>
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      accessibilityLabel={label}
    >
      <View style={{ flex: 1, gap: 4 }}>
        <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "500" }} numberOfLines={1}>
          {label}
          {item.playCount && item.playCount > 1 ? ` (x${item.playCount})` : ""}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>
          {formatDate(item.lastPlayedAt)}
        </Text>
        {item.durationMs && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>
            {formatDurationMs(item.positionMs)} / {formatDurationMs(item.durationMs)}
          </Text>
        )}
        <View style={{ flexDirection: "row", alignItems: "center", gap: 8, marginTop: 4 }}>
          <View style={[styles.progressBar, { backgroundColor: theme.colors.border }]}>
            <View style={[styles.progressFill, { width: `${pct}%`, backgroundColor: theme.colors.accent }]} />
          </View>
          <Text style={{ color: theme.colors.textMuted, fontSize: 14 }}>{Math.round(pct)}%</Text>
        </View>
      </View>
    </FocusableCard>
    {/* A remote has no swipe and no long-press, so the removal action has to be
        its own focusable sibling rather than a gesture on the row. Without it
        the mutation below was wired but unreachable. */}
    <FocusableCard
      onPress={onRemove}
      disabled={isPending}
      accessibilityLabel={t("auth.watchHistory.removeFromHistory", "Remove from history")}
      style={[styles.removeAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
    >
      <Text style={{ color: isPending ? theme.colors.textMuted : theme.colors.danger, fontSize: 18, fontWeight: "600" }}>
        {t("common.remove", "Remove")}
      </Text>
    </FocusableCard>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 16 },
  rowWrap: { flexDirection: "row", alignItems: "stretch", gap: 12 },
  row: { flex: 1, flexDirection: "row", alignItems: "center", padding: 20 },
  removeAction: { justifyContent: "center", paddingHorizontal: 24 },
  progressBar: { width: 120, height: 6, borderRadius: 3, overflow: "hidden" },
  progressFill: { height: "100%", borderRadius: 3 },
  emptyAction: { paddingHorizontal: 24, paddingVertical: 14 },
});
