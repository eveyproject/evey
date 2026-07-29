import { useMemo, useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueries } from "@tanstack/react-query";
import {
  usePlaybackServiceGetResumeItems,
  usePlaybackServiceGetNextUp,
  useLibraryServiceListMediaItems,
  useLibraryServiceListLibraries,
  getLibraryServiceListMediaItemsQueryOptions,
  useNotificationServiceGetUnreadCount,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";
import { CARD_WIDTH_FEATURED, POSTER_RATIO, GRID_GAP } from "../../../src/lib/tvLayout";
import { selectHomeLibraryRows, HOME_LIBRARY_ROW_PAGE_SIZE } from "../../../src/lib/homeLibraryRows";

const CARD_WIDTH = CARD_WIDTH_FEATURED;
const CARD_HEIGHT = CARD_WIDTH * POSTER_RATIO;
const CARD_TOTAL_WIDTH = CARD_WIDTH + GRID_GAP; // card + separator

const HERO_COUNT = 5;
const HERO_INTERVAL = 8000;
const HERO_HEIGHT = 480;

// rowItems/focusTarget hold the 3 static rows first, then one entry per
// library row, in the same order as libraryRows/libraryRowQueries.
const LIBRARY_ROWS_START = 3;

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

// The home screen must hand the TV remote's initial focus to exactly one
// element. The hero's Play button is the deliberate default (it's the
// screen's primary call to action); when there's no hero to show (no
// recently-added items), focus falls back to the first row that actually
// has items, in on-screen order, so the remote is never left with nowhere
// to go.
export function selectHomeFocusTarget(
  heroVisible: boolean,
  rowHasItems: boolean[],
): "hero" | number | null {
  if (heroVisible) return "hero";
  const idx = rowHasItems.findIndex(Boolean);
  return idx === -1 ? null : idx;
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
  const posterUri = item.id ? artworkUrl(serverUrl, item.id, "poster", CARD_WIDTH * 2) : undefined;
  const blurhash = (item.artwork ?? []).find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.blurhash;

  return (
    <FocusableCard
      onPress={() => router.push(`/media/${item.id}`)}
      hasTVPreferredFocus={isFirst}
      style={styles.card}
    >
      <View style={[styles.poster, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}>
        {posterUri ? (
          <View style={{ width: CARD_WIDTH, height: CARD_HEIGHT, borderRadius: theme.radii.sm, overflow: "hidden" }}>
            <Image
              source={{ uri: posterUri, headers: authHeaders }}
              style={{ width: CARD_WIDTH, height: CARD_HEIGHT }}
              contentFit="cover"
              transition={200}
              recyclingKey={item.id}
              placeholder={blurhash ? { blurhash } : undefined}
              placeholderContentFit="cover"
            />
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
  focusFirst,
}: {
  title: string;
  items: V1MediaItem[];
  serverUrl: string;
  authHeaders?: { Authorization: string };
  watchPercents?: Record<string, number>;
  focusFirst?: boolean;
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
            isFirst={index === 0 && !!focusFirst}
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
// screen - the 10-foot equivalent of the web/mobile hero, which we deliberately
// keep TV-only. Backdrop artwork sits behind the text/action content, faded
// into the background color so the title stays legible.
function HeroBanner({
  items,
  serverUrl,
  authHeaders,
  focusPlay,
}: {
  items: V1MediaItem[];
  serverUrl: string;
  authHeaders?: { Authorization: string };
  focusPlay?: boolean;
}) {
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
  const backdropBlurhash = (active?.artwork ?? []).find((a) => a.type === "ARTWORK_TYPE_BACKDROP")?.blurhash;

  // The hero's Play button jumps straight into the player, bypassing the
  // media detail screen where the parental PIN gate normally lives - so it
  // needs its own gate, keyed on whichever title is currently on screen.
  const { needsPin, profileId: gateProfileId } = useParentalGate(active?.officialRating);
  const [showPinModal, setShowPinModal] = useState(false);
  const [pendingPlayId, setPendingPlayId] = useState<string | null>(null);

  const handlePlay = useCallback(() => {
    if (!id) return;
    if (needsPin()) {
      setPendingPlayId(id);
      setShowPinModal(true);
      return;
    }
    router.push(`/player/${id}`);
  }, [id, needsPin]);

  const handlePinVerified = useCallback(() => {
    setShowPinModal(false);
    if (pendingPlayId) {
      router.push(`/player/${pendingPlayId}`);
      setPendingPlayId(null);
    }
  }, [pendingPlayId]);

  return (
    <View style={[styles.heroContainer, { height: HERO_HEIGHT, backgroundColor: theme.colors.surface }]}>
      {id ? (
        <Image
          key={id}
          source={{ uri: artworkUrl(serverUrl, id, "backdrop"), headers: authHeaders }}
          style={StyleSheet.absoluteFillObject}
          contentFit="cover"
          transition={400}
          placeholder={backdropBlurhash ? { blurhash: backdropBlurhash } : undefined}
          placeholderContentFit="cover"
        />
      ) : null}
      {/* Gradient-ish fade (stacked opacity layers) so the title/overview stay
          legible over any backdrop. */}
      <View style={[StyleSheet.absoluteFillObject, { flexDirection: "column", justifyContent: "flex-end" }]}>
        <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.15 }} />
        <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.4 }} />
        <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.75 }} />
        <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.95 }} />
      </View>
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
            onPress={handlePlay}
            hasTVPreferredFocus={focusPlay}
            style={[styles.heroPlayBtn, { backgroundColor: theme.colors.accent }]}
            accessibilityLabel={t("media.detail.play")}
          >
            <Text style={[styles.heroBtnLabel, { color: theme.colors.onAccent }]}>{t("media.detail.play")}</Text>
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
      {gateProfileId ? (
        <ParentalPINModal
          visible={showPinModal}
          onClose={() => { setShowPinModal(false); setPendingPlayId(null); }}
          onVerified={handlePinVerified}
          profileId={gateProfileId}
        />
      ) : null}
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

  // One "recently added" row per library, after the three transverse rows
  // above. selectHomeLibraryRows bounds the count (see its own doc comment)
  // so this never fires more than a handful of extra requests on the NAS-
  // class hardware Evey targets, however many libraries the server has.
  const libraries = useLibraryServiceListLibraries();
  const libraryRows = useMemo(
    () => selectHomeLibraryRows(libraries.data?.libraries ?? []),
    [libraries.data],
  );
  const libraryRowQueries = useQueries({
    queries: libraryRows.map((row) =>
      getLibraryServiceListMediaItemsQueryOptions({
        libraryId: row.id,
        sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
        sortOrder: "SORT_ORDER_DESC",
        "pagination.pageSize": HOME_LIBRARY_ROW_PAGE_SIZE,
      }),
    ),
  });

  const isLoading =
    resume.isLoading ||
    nextUp.isLoading ||
    recentlyAdded.isLoading ||
    libraries.isLoading ||
    libraryRowQueries.some((q) => q.isLoading);

  const watchPercents = useMemo(() => {
    const map: Record<string, number> = {};
    resume.data?.progress?.forEach((p) => {
      if (p.mediaItemId && p.percentComplete != null) {
        map[p.mediaItemId] = p.percentComplete;
      }
    });
    return map;
  }, [resume.data?.progress]);

  const recentlyAddedItems = recentlyAdded.data?.items ?? [];
  const rowItems = [
    resume.data?.items ?? [],
    nextUp.data?.items ?? [],
    recentlyAddedItems,
    ...libraryRowQueries.map((q) => q.data?.items ?? []),
  ];
  const focusTarget = selectHomeFocusTarget(
    recentlyAddedItems.length > 0,
    rowItems.map((items) => items.length > 0),
  );

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
            {t("notifications.unreadBanner", {
              count: unreadCount,
              defaultValue: `${unreadCount} unread notification${unreadCount !== 1 ? "s" : ""}`,
            })}
          </Text>
        </FocusableCard>
      )}
      <HeroBanner
        items={recentlyAddedItems}
        serverUrl={serverUrl}
        authHeaders={authHeaders}
        focusPlay={focusTarget === "hero"}
      />
      <PluginSlot name="home.section" />
      <MediaRow
        title={t("media.home.continueWatching")}
        items={rowItems[0]}
        serverUrl={serverUrl}
        authHeaders={authHeaders}
        watchPercents={watchPercents}
        focusFirst={focusTarget === 0}
      />
      <MediaRow
        title={t("media.home.upNext")}
        items={rowItems[1]}
        serverUrl={serverUrl}
        authHeaders={authHeaders}
        focusFirst={focusTarget === 1}
      />
      <MediaRow
        title={t("media.home.recentlyAdded")}
        items={rowItems[2]}
        serverUrl={serverUrl}
        authHeaders={authHeaders}
        focusFirst={focusTarget === 2}
      />
      {libraryRows.map((row, i) => (
        <MediaRow
          key={row.id}
          title={t("media.home.recentlyAddedInLibrary", { name: row.name })}
          items={rowItems[LIBRARY_ROWS_START + i]}
          serverUrl={serverUrl}
          authHeaders={authHeaders}
          focusFirst={focusTarget === LIBRARY_ROWS_START + i}
        />
      ))}
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
