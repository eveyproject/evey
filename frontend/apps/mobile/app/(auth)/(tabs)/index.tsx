import { ScrollView, View, RefreshControl } from "react-native";
import { Skeleton } from "../../../src/components/Skeleton";
import { useCallback, useMemo, useState } from "react";
import { Dimensions } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  usePlaybackServiceGetResumeItems,
  usePlaybackServiceGetNextUp,
  useFeedServiceRecentlyAdded,
  useFeedServiceRecommended,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { MediaRow } from "../../../src/components/MediaRow";
import { useCollections } from "../../../src/api/collections";
import { useMostPopular } from "../../../src/hooks/usePopular";
import { CollectionRow } from "../../../src/components/CollectionRow";

export default function HomeScreen() {
  const theme = useTheme();
  const { t } = useTranslation();

  const resume = usePlaybackServiceGetResumeItems({ limit: 20 });
  const nextUp = usePlaybackServiceGetNextUp({ limit: 20 });
  const recentlyAdded = useFeedServiceRecentlyAdded();
  const recommended = useFeedServiceRecommended({ limit: 20 });
  const popular = useMostPopular(20);
  const collections = useCollections();

  const [refreshing, setRefreshing] = useState(false);
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

  if (isLoading && !refreshing) {
    const skeletonW = Dimensions.get("window").width * 0.3;
    const skeletonH = skeletonW * 1.5;
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.bg, paddingTop: theme.spacing.lg }}>
        {Array.from({ length: 5 }).map((_, row) => (
          <View key={row} style={{ marginBottom: theme.spacing.lg }}>
            <Skeleton width={120} height={18} borderRadius={4} style={{ marginLeft: theme.spacing.md, marginBottom: theme.spacing.sm }} />
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: theme.spacing.md }}>
              {Array.from({ length: 3 }).map((_, col) => (
                <View key={col} style={{ marginRight: theme.spacing.sm }}>
                  <Skeleton width={skeletonW} height={skeletonH} borderRadius={theme.radii.md} />
                  <Skeleton width={skeletonW * 0.7} height={13} borderRadius={4} style={{ marginTop: 4 }} />
                </View>
              ))}
            </ScrollView>
          </View>
        ))}
      </View>
    );
  }

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingTop: theme.spacing.lg, paddingBottom: theme.spacing.xl }}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      {/* Resume Playback — same as web resume_playback */}
      <MediaRow
        title={t("common.feeds.labels.resume_playback")}
        items={resume.data?.items ?? []}
        onItemPress={handleItemPress}
        watchPercents={watchPercents}
      />
      {/* Next Up — same as web next_up */}
      <MediaRow
        title={t("media.home.upNext")}
        items={nextUp.data?.items ?? []}
        onItemPress={handleItemPress}
      />
      {/* Collections — same as web collections */}
      <CollectionRow
        title={t("media.home.collections")}
        collections={collections.data?.collections ?? []}
      />
      {/* Recommended — same as web recommended */}
      <MediaRow
        title={t("media.home.recommendedForYou")}
        items={recommendedItems}
        onItemPress={handleItemPress}
      />
      {/* Most Popular — same as web most_popular */}
      <MediaRow
        title={t("media.home.mostPopular")}
        items={popularItems}
        onItemPress={handleItemPress}
      />
      {/* Recently Added — feed-based like web hero data */}
      <MediaRow
        title={t("media.home.recentlyAdded")}
        items={recentItems}
        onItemPress={handleItemPress}
      />
    </ScrollView>
  );
}
