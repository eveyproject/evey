import { ScrollView, View, RefreshControl } from "react-native";
import { Skeleton } from "../../../src/components/Skeleton";
import { useCallback, useMemo, useState } from "react";
import { Dimensions } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  usePlaybackServiceGetResumeItems,
  usePlaybackServiceGetNextUp,
  useLibraryServiceListMediaItems,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { MediaRow } from "../../../src/components/MediaRow";

export default function HomeScreen() {
  const theme = useTheme();
  const { t } = useTranslation();

  const resume = usePlaybackServiceGetResumeItems({ limit: 20 });
  const nextUp = usePlaybackServiceGetNextUp({ limit: 20 });
  const recentlyAdded = useLibraryServiceListMediaItems({
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
    sortOrder: "SORT_ORDER_DESC",
    "pagination.pageSize": 20,
  });

  const [refreshing, setRefreshing] = useState(false);
  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([resume.refetch(), nextUp.refetch(), recentlyAdded.refetch()]);
    setRefreshing(false);
  }, [resume, nextUp, recentlyAdded]);

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

  function handleItemPress(item: V1MediaItem) {
    router.push(`/media/${item.id}`);
  }

  if (isLoading && !refreshing) {
    const skeletonW = Dimensions.get("window").width * 0.3;
    const skeletonH = skeletonW * 1.5;
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.bg, paddingTop: theme.spacing.lg }}>
        {Array.from({ length: 3 }).map((_, row) => (
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
      <MediaRow
        title={t("media.home.continueWatching")}
        items={resume.data?.items ?? []}
        onItemPress={handleItemPress}
        watchPercents={watchPercents}
      />
      <MediaRow
        title={t("media.home.upNext")}
        items={nextUp.data?.items ?? []}
        onItemPress={handleItemPress}
      />
      <MediaRow
        title={t("media.home.recentlyAdded")}
        items={recentlyAdded.data?.items ?? []}
        onItemPress={handleItemPress}
      />
    </ScrollView>
  );
}
