import { useMemo, useState, useEffect } from "react";
import { View, Text, ScrollView, FlatList, ActivityIndicator, StyleSheet } from "react-native";
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

const HERO_COUNT = 5;
const HERO_INTERVAL = 8000;
const HERO_HEIGHT = 480;

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

// HeroBanner spotlights the most recently added titles at the top of the home
// screen — the 10-foot equivalent of the web/mobile hero, which we deliberately
// keep TV-only. It is text-and-action driven (big title, genres, overview, Play
// / More info) so it carries its weight even though the TV app has no image
// renderer yet: once expo-image lands, drop the item's backdrop behind `content`.
function HeroBanner({ items }: { items: V1MediaItem[] }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const heroItems = items.slice(0, HERO_COUNT);
  const [activeIndex, setActiveIndex] = useState(0);

  useEffect(() => {
    if (heroItems.length <= 1) return;
    const id = setInterval(() => {
      setActiveIndex((prev) => (prev + 1) % heroItems.length);
    }, HERO_INTERVAL);
    return () => clearInterval(id);
  }, [heroItems.length]);

  if (heroItems.length === 0) return null;

  const active = heroItems[activeIndex] ?? heroItems[0];
  const id = active?.id ?? "";
  const genres = (active?.genres ?? []).slice(0, 3);
  const metaLine = [active?.year ? String(active.year) : "", genres.join(" · ")]
    .filter(Boolean)
    .join("   ·   ");

  return (
    <View style={[styles.heroContainer, { height: HERO_HEIGHT, backgroundColor: theme.colors.surface }]}>
      {/* Backdrop placeholder: the TV app ships no image renderer yet (see
          MediaCard), so we render a flat surface. Swap for the item's backdrop
          artwork here once expo-image is available on TV. */}
      <View style={styles.heroContent}>
        <Text style={[styles.heroTitle, { color: theme.colors.text }]} numberOfLines={2}>
          {active?.title}
        </Text>
        {metaLine ? (
          <Text style={[styles.heroMeta, { color: theme.colors.textMuted }]}>{metaLine}</Text>
        ) : null}
        {active?.overview ? (
          <Text style={[styles.heroOverview, { color: theme.colors.textMuted }]} numberOfLines={3}>
            {active.overview}
          </Text>
        ) : null}
        <View style={styles.heroButtons}>
          <FocusableCard
            onPress={() => id && router.push(`/player/${id}`)}
            hasTVPreferredFocus
            style={[styles.heroPlayBtn, { backgroundColor: theme.colors.accent }]}
            accessibilityLabel={t("media.detail.play")}
          >
            <Text style={[styles.heroBtnLabel, { color: theme.colors.bg }]}>{t("media.detail.play")}</Text>
          </FocusableCard>
          <FocusableCard
            onPress={() => id && router.push(`/media/${id}`)}
            style={[styles.heroInfoBtn, { backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface }]}
            accessibilityLabel={t("media.home.moreInfo")}
          >
            <Text style={[styles.heroBtnLabel, { color: theme.colors.text }]}>{t("media.home.moreInfo")}</Text>
          </FocusableCard>
        </View>
        {heroItems.length > 1 && (
          <View style={styles.heroDots}>
            {heroItems.map((it, i) => (
              <View
                key={it.id ?? i}
                style={[
                  styles.heroDot,
                  { backgroundColor: i === activeIndex ? theme.colors.accent : theme.colors.border },
                ]}
              />
            ))}
          </View>
        )}
      </View>
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
      <HeroBanner items={recentlyAdded.data?.items ?? []} />
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
  heroContainer: {
    width: "100%",
    justifyContent: "flex-end",
    overflow: "hidden",
    marginBottom: 32,
  },
  heroContent: {
    paddingHorizontal: 48,
    paddingBottom: 48,
    maxWidth: 1100,
  },
  heroTitle: {
    fontSize: 48,
    fontWeight: "800",
  },
  heroMeta: {
    fontSize: 20,
    marginTop: 12,
  },
  heroOverview: {
    fontSize: 20,
    lineHeight: 28,
    marginTop: 12,
  },
  heroButtons: {
    flexDirection: "row",
    gap: 16,
    marginTop: 24,
  },
  heroPlayBtn: {
    paddingVertical: 14,
    paddingHorizontal: 36,
    justifyContent: "center",
    alignItems: "center",
  },
  heroInfoBtn: {
    paddingVertical: 14,
    paddingHorizontal: 36,
    justifyContent: "center",
    alignItems: "center",
  },
  heroBtnLabel: {
    fontSize: 22,
    fontWeight: "700",
  },
  heroDots: {
    flexDirection: "row",
    gap: 10,
    marginTop: 24,
  },
  heroDot: {
    width: 12,
    height: 12,
    borderRadius: 6,
  },
});
