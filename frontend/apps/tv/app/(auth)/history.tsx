import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  usePlaybackServiceGetWatchHistory,
  usePlaybackServiceMarkAsUnwatched,
  getPlaybackServiceGetWatchHistoryQueryKey,
} from "@evey/api";
import type { V1WatchProgress } from "@evey/api";
import { useTheme } from "../../src/providers/ThemeProvider";
import { FocusableCard } from "../../src/components/FocusableCard";

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

export default function HistoryScreen() {
  const theme = useTheme();
  const qc = useQueryClient();

  const { data, isLoading } = usePlaybackServiceGetWatchHistory({ limit: 50 });
  const items = data?.items ?? [];

  const unwatch = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
      },
    },
  });

  if (isLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>Watch History</Text>

      {items.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>No watch history</Text>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(item) => item.id ?? item.mediaItemId ?? ""}
          contentContainerStyle={{ paddingHorizontal: 24, paddingBottom: 64 }}
          ItemSeparatorComponent={() => <View style={{ height: 4 }} />}
          renderItem={({ item, index }) => (
            <HistoryItem
              item={item}
              theme={theme}
              isFirst={index === 0}
              onPress={() => item.mediaItemId && router.push(`/media/${item.mediaItemId}`)}
              onRemove={() => item.mediaItemId && unwatch.mutate({ mediaItemId: item.mediaItemId })}
              isPending={unwatch.isPending}
            />
          )}
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
  const pct = item.percentComplete ?? 0;

  return (
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
    >
      <View style={{ flex: 1, gap: 4 }}>
        <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "500" }} numberOfLines={1}>
          {item.mediaItemId?.slice(0, 8)}
          {item.playCount && item.playCount > 1 ? ` (x${item.playCount})` : ""}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>
          {formatDate(item.lastPlayedAt)}
        </Text>
        {item.durationMs && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>
            {formatDuration(item.positionMs)} / {formatDuration(item.durationMs)}
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
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 16 },
  row: { flexDirection: "row", alignItems: "center", padding: 20 },
  progressBar: { width: 120, height: 6, borderRadius: 3, overflow: "hidden" },
  progressFill: { height: "100%", borderRadius: 3 },
});
