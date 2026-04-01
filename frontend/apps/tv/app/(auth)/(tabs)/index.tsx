import { useCallback, useMemo } from "react";
import { View, Text, ScrollView, FlatList, ActivityIndicator, StyleSheet, Dimensions } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  usePlaybackServiceGetResumeItems,
  usePlaybackServiceGetNextUp,
  useLibraryServiceListMediaItems,
  useNotificationServiceGetUnreadCount,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { FocusableCard } from "../../../src/components/FocusableCard";

const CARD_WIDTH = 220;
const CARD_HEIGHT = 330;
const CARD_TOTAL_WIDTH = CARD_WIDTH + 16; // card + separator

const tvRowGetItemLayout = (_: unknown, index: number) => ({
  length: CARD_TOTAL_WIDTH,
  offset: CARD_TOTAL_WIDTH * index,
  index,
});

function tvRowKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

function RowSeparator() {
  return <View style={{ width: 16 }} />;
}

function MediaCard({
  item,
  serverUrl,
  authHeaders,
  isFirst,
  watchPercent,
}: {
  item: V1MediaItem;
  serverUrl: string;
  authHeaders?: { Authorization: string };
  isFirst?: boolean;
  watchPercent?: number;
}) {
  const theme = useTheme();
  const posterUri = item.id ? artworkUrl(serverUrl, item.id) : undefined;

  return (
    <FocusableCard
      onPress={() => router.push(`/media/${item.id}`)}
      hasTVPreferredFocus={isFirst}
      style={styles.card}
    >
      <View style={[styles.poster, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}>
        {posterUri ? (
          <View style={{ width: CARD_WIDTH, height: CARD_HEIGHT, borderRadius: theme.radii.sm, overflow: "hidden" }}>
            {/* expo-image not installed in TV, use a colored placeholder */}
            <View style={{ flex: 1, backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface }} />
          </View>
        ) : (
          <View style={[styles.posterPlaceholder, { backgroundColor: theme.colors.surface }]}>
            <Text style={{ color: theme.colors.textMuted }}>?</Text>
          </View>
        )}
      </View>
      {watchPercent != null && watchPercent > 0 && (
        <View style={[styles.progressBar, { backgroundColor: theme.colors.border }]}>
          <View style={[styles.progressFill, { width: `${watchPercent}%`, backgroundColor: theme.colors.accent }]} />
        </View>
      )}
      <Text style={[styles.cardTitle, { color: theme.colors.text }]} numberOfLines={1}>
        {item.title}
      </Text>
      {item.year && (
        <Text style={[styles.cardYear, { color: theme.colors.textMuted }]}>{item.year}</Text>
      )}
    </FocusableCard>
  );
}

function MediaRow({
  title,
  items,
  serverUrl,
  authHeaders,
  watchPercents,
}: {
  title: string;
  items: V1MediaItem[];
  serverUrl: string;
  authHeaders?: { Authorization: string };
  watchPercents?: Record<string, number>;
}) {
  const theme = useTheme();
  if (items.length === 0) return null;

  return (
    <View style={styles.rowContainer}>
      <Text style={[styles.rowTitle, { color: theme.colors.text }]}>{title}</Text>
      <FlatList
        horizontal
        data={items}
        keyExtractor={tvRowKeyExtractor}
        renderItem={({ item, index }) => (
          <MediaCard
            item={item}
            serverUrl={serverUrl}
            authHeaders={authHeaders}
            isFirst={index === 0}
            watchPercent={item.id ? watchPercents?.[item.id] : undefined}
          />
        )}
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{ paddingHorizontal: 24 }}
        ItemSeparatorComponent={RowSeparator}
        getItemLayout={tvRowGetItemLayout}
        initialNumToRender={5}
        maxToRenderPerBatch={8}
        windowSize={5}
      />
    </View>
  );
}

export default function HomeScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const { data: unreadData } = useNotificationServiceGetUnreadCount();
  const unreadCount = unreadData?.count ?? 0;

  const resume = usePlaybackServiceGetResumeItems({ limit: 20 });
  const nextUp = usePlaybackServiceGetNextUp({ limit: 20 });
  const recentlyAdded = useLibraryServiceListMediaItems({
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
    sortOrder: "SORT_ORDER_DESC",
    "pagination.pageSize": 20,
  });

  const isLoading = resume.isLoading || nextUp.isLoading || recentlyAdded.isLoading;

  const watchPercents = useMemo(() => {
    const map: Record<string, number> = {};
    resume.data?.progress?.forEach((p) => {
      if (p.mediaItemId && p.percentComplete != null) {
        map[p.mediaItemId] = p.percentComplete;
      }
    });
    return map;
  }, [resume.data?.progress]);

  if (isLoading) {
    return (
      <View style={[styles.loading, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ paddingVertical: 32 }}>
      {unreadCount > 0 && (
        <FocusableCard
          onPress={() => router.push("/(auth)/notifications")}
          style={styles.notifBanner}
        >
          <Text style={{ color: theme.colors.accent, fontSize: 20, fontWeight: "600" }}>
            {unreadCount} unread notification{unreadCount !== 1 ? "s" : ""}
          </Text>
        </FocusableCard>
      )}
      <MediaRow
        title={t("media.home.continueWatching")}
        items={resume.data?.items ?? []}
        serverUrl={serverUrl}
        authHeaders={authHeaders}
        watchPercents={watchPercents}
      />
      <MediaRow
        title={t("media.home.upNext")}
        items={nextUp.data?.items ?? []}
        serverUrl={serverUrl}
        authHeaders={authHeaders}
      />
      <MediaRow
        title={t("media.home.recentlyAdded")}
        items={recentlyAdded.data?.items ?? []}
        serverUrl={serverUrl}
        authHeaders={authHeaders}
      />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  loading: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  notifBanner: {
    marginHorizontal: 24,
    marginBottom: 16,
    paddingVertical: 12,
    paddingHorizontal: 24,
  },
  rowContainer: {
    marginBottom: 32,
  },
  rowTitle: {
    fontSize: 28,
    fontWeight: "700",
    paddingHorizontal: 24,
    marginBottom: 16,
  },
  card: {
    width: CARD_WIDTH,
  },
  poster: {
    width: CARD_WIDTH,
    height: CARD_HEIGHT,
    overflow: "hidden",
  },
  posterPlaceholder: {
    width: CARD_WIDTH,
    height: CARD_HEIGHT,
    justifyContent: "center",
    alignItems: "center",
  },
  progressBar: {
    height: 4,
    borderRadius: 2,
    marginTop: 4,
    overflow: "hidden",
  },
  progressFill: {
    height: "100%",
    borderRadius: 2,
  },
  cardTitle: {
    fontSize: 16,
    fontWeight: "600",
    marginTop: 8,
  },
  cardYear: {
    fontSize: 14,
    marginTop: 2,
  },
});
