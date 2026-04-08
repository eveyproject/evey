import { ScrollView, View, RefreshControl, Text, Pressable, Modal, Switch, StyleSheet, Dimensions } from "react-native";
import { LinearGradient } from "expo-linear-gradient";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { Skeleton } from "../../../src/components/Skeleton";
import { useCallback, useMemo, useState, useEffect, useRef } from "react";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import {
  usePlaybackServiceGetResumeItems,
  usePlaybackServiceGetNextUp,
  useFeedServiceRecentlyAdded,
  useFeedServiceRecommended,
  useFeedServiceGetFeedConfig,
  useFeedServiceUpdateProfileFeedConfig,
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  getLibraryServiceGetMediaItemQueryOptions,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";

interface FeedConfig {
  feedType?: string;
  enabled?: boolean;
  displayOrder?: number;
}
import { useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { MediaRow } from "../../../src/components/MediaRow";
import { useCollections } from "../../../src/api/collections";
import { useMostPopular } from "../../../src/hooks/usePopular";
import { CollectionRow } from "../../../src/components/CollectionRow";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";

const SCREEN_WIDTH = Dimensions.get("window").width;
const SCREEN_HEIGHT = Dimensions.get("window").height;
const HERO_HEIGHT = Math.round(Math.max(SCREEN_HEIGHT * 0.45, 280));
const HERO_INTERVAL = 8000;
const HERO_COUNT = 5;

function HeroBanner() {
  const theme = useTheme();
  const { t } = useTranslation();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();
  const { data } = useFeedServiceRecentlyAdded({ daysWindow: 30, limit: 20 }, { query: { staleTime: 5 * 60_000 } });
  const candidates = (data?.items ?? []).filter((i) => !!i.mediaItemId).slice(0, HERO_COUNT);
  const heroIds = candidates.map((c) => c.mediaItemId!);

  const details = useQueries({
    queries: heroIds.map((id) => ({
      ...getLibraryServiceGetMediaItemQueryOptions(id),
      staleTime: 5 * 60_000,
    })),
  });

  const [activeIndex, setActiveIndex] = useState(0);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (heroIds.length <= 1) return;
    timerRef.current = setInterval(() => {
      setActiveIndex((prev) => (prev + 1) % heroIds.length);
    }, HERO_INTERVAL);
    return () => { if (timerRef.current) clearInterval(timerRef.current); };
  }, [heroIds.length]);

  if (heroIds.length === 0) return null;

  const activeId = heroIds[activeIndex] ?? "";
  const detail = details[activeIndex]?.data;
  const backdropUrl = `${serverUrl}/api/v1/media/${activeId}/artwork/backdrop`;
  const displayTitle = candidates[activeIndex]?.title ?? detail?.title ?? "";
  const genres = (detail?.genres ?? []).slice(0, 3);
  const overview = detail?.overview;

  return (
    <View style={[heroStyles.container, { height: HERO_HEIGHT }]}>
      <Image
        source={{ uri: backdropUrl, headers: authHeaders }}
        style={StyleSheet.absoluteFillObject}
        contentFit="cover"
        transition={400}
      />
      {/* Bottom gradient for text readability */}
      <LinearGradient
        colors={["transparent", "rgba(0,0,0,0.7)", "rgba(0,0,0,0.95)"]}
        locations={[0.3, 0.7, 1]}
        style={StyleSheet.absoluteFillObject}
        pointerEvents="none"
      />
      {/* Left gradient (like web) */}
      <LinearGradient
        colors={["rgba(0,0,0,0.7)", "rgba(0,0,0,0.2)", "transparent"]}
        locations={[0, 0.5, 1]}
        start={{ x: 0, y: 0.5 }}
        end={{ x: 0.6, y: 0.5 }}
        style={StyleSheet.absoluteFillObject}
        pointerEvents="none"
      />
      {/* Content */}
      <View style={heroStyles.content}>
        <Text style={heroStyles.title} numberOfLines={2}>{displayTitle}</Text>
        {genres.length > 0 && (
          <Text style={[heroStyles.meta, { color: theme.colors.textMuted }]}>
            {genres.join(" · ")}
          </Text>
        )}
        {overview ? (
          <Text style={[heroStyles.overview, { color: "rgba(255,255,255,0.8)" }]} numberOfLines={2}>
            {overview}
          </Text>
        ) : null}
        <View style={heroStyles.buttons}>
          <Pressable
            style={[heroStyles.playBtn, { backgroundColor: "#fff" }]}
            onPress={() => activeId && router.push(`/media/${activeId}`)}
            accessible
            accessibilityRole="button"
            accessibilityLabel={t("media.detail.play")}
          >
            <Ionicons name="play" size={18} color="#111" />
            <Text style={{ color: "#111", fontWeight: "700", fontSize: 14 }}>{t("media.detail.play")}</Text>
          </Pressable>
          <Pressable
            style={[heroStyles.infoBtn, { borderColor: "rgba(255,255,255,0.6)" }]}
            onPress={() => activeId && router.push(`/media/${activeId}`)}
            accessible
            accessibilityRole="button"
            accessibilityLabel={t("media.home.moreInfo")}
          >
            <Ionicons name="information-circle-outline" size={18} color="#fff" />
            <Text style={{ color: "#fff", fontWeight: "600", fontSize: 14 }}>{t("media.home.moreInfo")}</Text>
          </Pressable>
        </View>
      </View>
      {/* Dots */}
      {heroIds.length > 1 && (
        <View style={heroStyles.dots}>
          {heroIds.map((_, i) => (
            <Pressable key={i} onPress={() => setActiveIndex(i)}>
              <View style={[
                heroStyles.dot,
                i === activeIndex ? { backgroundColor: "#fff", width: 16 } : { backgroundColor: "rgba(255,255,255,0.4)" },
              ]} />
            </Pressable>
          ))}
        </View>
      )}
    </View>
  );
}

const heroStyles = StyleSheet.create({
  container: { width: "100%", overflow: "hidden" },
  overlay: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: "transparent",
  },
  content: {
    position: "absolute",
    bottom: 40,
    left: 16,
    right: 16,
  },
  title: {
    color: "#fff",
    fontSize: 22,
    fontWeight: "800",
    textShadowColor: "rgba(0,0,0,0.8)",
    textShadowOffset: { width: 0, height: 1 },
    textShadowRadius: 4,
  },
  meta: {
    fontSize: 12,
    marginTop: 4,
    textShadowColor: "rgba(0,0,0,0.8)",
    textShadowOffset: { width: 0, height: 1 },
    textShadowRadius: 4,
  },
  overview: {
    fontSize: 12,
    marginTop: 4,
    lineHeight: 16,
    textShadowColor: "rgba(0,0,0,0.8)",
    textShadowOffset: { width: 0, height: 1 },
    textShadowRadius: 4,
  },
  buttons: {
    flexDirection: "row",
    gap: 10,
    marginTop: 12,
  },
  playBtn: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    paddingVertical: 8,
    paddingHorizontal: 16,
    borderRadius: 6,
  },
  infoBtn: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    paddingVertical: 8,
    paddingHorizontal: 16,
    borderRadius: 6,
    borderWidth: 1,
  },
  dots: {
    position: "absolute",
    bottom: 12,
    alignSelf: "center",
    flexDirection: "row",
    gap: 6,
    alignItems: "center",
  },
  dot: {
    height: 4,
    width: 8,
    borderRadius: 2,
  },
});

function LibraryFeedRow({ libraryId, name, libraryType, onItemPress }: { libraryId: string; name: string; libraryType?: string; onItemPress: (item: V1MediaItem) => void }) {
  const typeFilter = libraryType === "LIBRARY_TYPE_SHOWS"
    ? "MEDIA_ITEM_TYPE_SERIES"
    : libraryType === "LIBRARY_TYPE_MOVIES"
      ? "MEDIA_ITEM_TYPE_MOVIE"
      : undefined;
  const { data, isLoading } = useLibraryServiceListMediaItems({
    libraryId,
    ...(typeFilter ? { type: typeFilter as never } : {}),
    "pagination.pageSize": 20,
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED" as never,
    sortOrder: "SORT_ORDER_DESC" as never,
  });

  const items = useMemo(
    () => (data?.items ?? []).filter((i): i is V1MediaItem => !!i.id),
    [data?.items],
  );

  if (!isLoading && items.length === 0) return null;

  return <MediaRow title={name} items={items} onItemPress={onItemPress} />;
}

const FEED_TYPES = [
  "resume_playback",
  "next_episode",
  "collections",
  "recommended",
  "most_popular",
  "recently_added",
] as const;

const FEED_LABELS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  continue_watching: "common.feeds.labels.resume_playback",
  next_episode: "media.home.upNext",
  collections: "media.home.collections",
  recommended: "media.home.recommendedForYou",
  most_popular: "media.home.mostPopular",
  recently_added: "media.home.recentlyAdded",
};

function FeedCustomizeModal({
  visible,
  onClose,
  configs,
  profileId,
  libraryNames,
}: {
  visible: boolean;
  onClose: () => void;
  configs: FeedConfig[];
  profileId: string;
  libraryNames: Record<string, string>;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const updateFeed = useFeedServiceUpdateProfileFeedConfig();

  const [local, setLocal] = useState<FeedConfig[]>([]);

  useEffect(() => {
    if (visible) {
      const sorted = [...configs].sort((a, b) => (a.displayOrder ?? 0) - (b.displayOrder ?? 0));
      if (sorted.length === 0) {
        setLocal(FEED_TYPES.map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: i })));
      } else {
        setLocal(sorted);
      }
    }
  }, [visible, configs]);

  const persist = useCallback(async (updated: FeedConfig[]) => {
    for (const c of updated) {
      if (!c.feedType) continue;
      await updateFeed.mutateAsync({
        feedType: c.feedType,
        data: { profileId, enabled: c.enabled, displayOrder: c.displayOrder },
      }).catch(() => {});
    }
    await qc.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] });
  }, [updateFeed, profileId, qc]);

  const toggle = (idx: number) => {
    setLocal((prev) => {
      const next = prev.map((c, i) => (i === idx ? { ...c, enabled: !c.enabled } : c));
      persist(next);
      return next;
    });
  };

  const move = (idx: number, dir: -1 | 1) => {
    const target = idx + dir;
    if (target < 0 || target >= local.length) return;
    setLocal((prev) => {
      const arr = [...prev];
      [arr[idx], arr[target]] = [arr[target], arr[idx]];
      const next = arr.map((c, i) => ({ ...c, displayOrder: i }));
      persist(next);
      return next;
    });
  };

  const feedLabel = (feedType: string) => {
    if (feedType.startsWith("library:")) {
      const libId = feedType.replace("library:", "");
      return libraryNames[libId] || libId;
    }
    return t(FEED_LABELS[feedType] ?? feedType);
  };

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <View style={[feedStyles.backdrop, { backgroundColor: "rgba(0,0,0,0.5)" }]}>
        <View style={[feedStyles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[feedStyles.sheetTitle, { color: theme.colors.text }]}>
            {t("media.home.customizeFeed") || "Customize Feed"}
          </Text>
          <ScrollView style={{ maxHeight: 400 }}>
            {local.map((c, i) => (
              <View key={c.feedType} style={[feedStyles.row, { borderBottomColor: theme.colors.border }]}>
                <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
                  <Pressable onPress={() => move(i, -1)} disabled={i === 0} hitSlop={8}>
                    <Ionicons name="chevron-up" size={18} color={i === 0 ? theme.colors.border : theme.colors.textMuted} />
                  </Pressable>
                  <Pressable onPress={() => move(i, 1)} disabled={i === local.length - 1} hitSlop={8}>
                    <Ionicons name="chevron-down" size={18} color={i === local.length - 1 ? theme.colors.border : theme.colors.textMuted} />
                  </Pressable>
                </View>
                <Text style={[feedStyles.feedLabel, { color: theme.colors.text }]}>
                  {feedLabel(c.feedType ?? "")}
                </Text>
                <Switch
                  value={c.enabled}
                  onValueChange={() => toggle(i)}
                  trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                />
              </View>
            ))}
          </ScrollView>
          <Pressable onPress={onClose} style={[feedStyles.cancelBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md, marginTop: 16 }]}>
            <Text style={{ color: theme.colors.text, fontWeight: "500", fontSize: 15 }}>{t("common.close") || "Close"}</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const feedStyles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "flex-end" },
  sheet: { padding: 20, paddingBottom: 32 },
  sheetTitle: { fontSize: 18, fontWeight: "700", marginBottom: 16 },
  row: { flexDirection: "row", alignItems: "center", paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth, gap: 12 },
  feedLabel: { flex: 1, fontSize: 14, fontWeight: "500" },
  saveBtn: { paddingVertical: 12, alignItems: "center" },
  cancelBtn: { paddingVertical: 12, alignItems: "center" },
});

export default function HomeScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId) ?? "";

  const resume = usePlaybackServiceGetResumeItems({ limit: 20 });
  const nextUp = usePlaybackServiceGetNextUp({ limit: 20 });
  const recentlyAdded = useFeedServiceRecentlyAdded({ daysWindow: 30, limit: 20 });
  const recommended = useFeedServiceRecommended({ limit: 20 });
  const popular = useMostPopular(20);
  const collections = useCollections();
  const { data: feedConfigData } = useFeedServiceGetFeedConfig({ profileId });
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraryNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const lib of librariesData?.libraries ?? []) {
      if (lib.id && lib.name) map[lib.id] = lib.name;
    }
    return map;
  }, [librariesData]);

  const [refreshing, setRefreshing] = useState(false);
  const [showCustomize, setShowCustomize] = useState(false);

  const feedConfigs = useMemo(() => {
    const configs = feedConfigData?.configs ?? [];
    if (configs.length === 0) {
      return FEED_TYPES.map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: i }));
    }
    return [...configs].sort((a, b) => (a.displayOrder ?? 0) - (b.displayOrder ?? 0));
  }, [feedConfigData]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([
      resume.refetch(),
      nextUp.refetch(),
      recentlyAdded.refetch(),
      recommended.refetch(),
      popular.refetch(),
      collections.refetch(),
    ]);
    setRefreshing(false);
  }, [resume, nextUp, recentlyAdded, recommended, popular, collections]);

  const isLoading =
    resume.isLoading ||
    nextUp.isLoading ||
    recentlyAdded.isLoading ||
    recommended.isLoading ||
    popular.isLoading ||
    collections.isLoading;

  const watchPercents = useMemo(() => {
    const map: Record<string, number> = {};
    resume.data?.progress?.forEach((p) => {
      if (p.mediaItemId && p.percentComplete != null) {
        map[p.mediaItemId] = p.percentComplete;
      }
    });
    return map;
  }, [resume.data?.progress]);

  const recentItems = useMemo(
    () =>
      (recentlyAdded.data?.items ?? [])
        .filter((i) => !!i.mediaItemId)
        .map((i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem),
    [recentlyAdded.data?.items],
  );

  const recommendedItems = useMemo(
    () =>
      (recommended.data?.items ?? [])
        .filter((i) => !!i.mediaItemId)
        .map((i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem),
    [recommended.data?.items],
  );

  const popularItems = useMemo(
    () =>
      (popular.data ?? []).map(
        (i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem,
      ),
    [popular.data],
  );

  function handleItemPress(item: V1MediaItem) {
    router.push(`/media/${item.id}`);
  }

  function renderSection(feedType: string) {
    switch (feedType) {
      case "resume_playback":
      case "continue_watching":
        return <MediaRow key={feedType} title={t("common.feeds.labels.resume_playback")} items={resume.data?.items ?? []} onItemPress={handleItemPress} watchPercents={watchPercents} />;
      case "next_episode":
        return <MediaRow key={feedType} title={t("media.home.upNext")} items={nextUp.data?.items ?? []} onItemPress={handleItemPress} />;
      case "collections":
        return <CollectionRow key={feedType} title={t("media.home.collections")} collections={collections.data?.collections ?? []} />;
      case "recommended":
        return <MediaRow key={feedType} title={t("media.home.recommendedForYou")} items={recommendedItems} onItemPress={handleItemPress} />;
      case "most_popular":
        return <MediaRow key={feedType} title={t("media.home.mostPopular")} items={popularItems} onItemPress={handleItemPress} />;
      case "recently_added":
        return <MediaRow key={feedType} title={t("media.home.recentlyAdded")} items={recentItems} onItemPress={handleItemPress} />;
      default:
        if (feedType.startsWith("library:")) {
          const libId = feedType.replace("library:", "");
          const lib = (librariesData?.libraries ?? []).find((l) => l.id === libId);
          if (!lib) return null;
          return <LibraryFeedRow key={feedType} libraryId={libId} name={lib.name ?? ""} libraryType={lib.type} onItemPress={handleItemPress} />;
        }
        return null;
    }
  }

  if (isLoading && !refreshing) {
    const skeletonW = SCREEN_WIDTH * 0.35;
    const skeletonH = skeletonW * 1.5;
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
        <Skeleton width={SCREEN_WIDTH} height={HERO_HEIGHT} borderRadius={0} />
        <View style={{ paddingTop: theme.spacing.lg }}>
          {Array.from({ length: 4 }).map((_, row) => (
            <View key={row} style={{ marginBottom: theme.spacing.lg }}>
              <Skeleton width={140} height={18} borderRadius={4} style={{ marginLeft: theme.spacing.md, marginBottom: theme.spacing.sm }} />
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: theme.spacing.md }}>
                {Array.from({ length: 4 }).map((_, col) => (
                  <View key={col} style={{ marginRight: theme.spacing.sm }}>
                    <Skeleton width={skeletonW} height={skeletonH} borderRadius={theme.radii.md} />
                    <Skeleton width={skeletonW * 0.7} height={13} borderRadius={4} style={{ marginTop: 4 }} />
                  </View>
                ))}
              </ScrollView>
            </View>
          ))}
        </View>
      </View>
    );
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <ScrollView
        style={{ flex: 1 }}
        contentContainerStyle={{ paddingBottom: theme.spacing.xl }}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
      >
        <HeroBanner />
        <View style={{ paddingTop: theme.spacing.md }}>
          <Pressable
            onPress={() => setShowCustomize(true)}
            style={{ alignSelf: "flex-end", marginRight: theme.spacing.md, marginBottom: theme.spacing.sm, flexDirection: "row", alignItems: "center", gap: 4 }}
          >
            <Ionicons name="options-outline" size={16} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("media.home.customize") || "Customize"}</Text>
          </Pressable>
          {feedConfigs.filter((c) => c.enabled !== false).map((c) => renderSection(c.feedType ?? ""))}
          {(librariesData?.libraries ?? [])
            .filter((lib) => lib.id && !feedConfigs.some((c) => c.feedType === `library:${lib.id}`))
            .map((lib) => (
              <LibraryFeedRow key={`library:${lib.id}`} libraryId={lib.id!} name={lib.name ?? ""} libraryType={lib.type} onItemPress={handleItemPress} />
            ))}
        </View>
      </ScrollView>
      <FeedCustomizeModal
        visible={showCustomize}
        onClose={() => setShowCustomize(false)}
        configs={feedConfigData?.configs ?? []}
        profileId={profileId}
        libraryNames={libraryNames}
      />
    </View>
  );
}

