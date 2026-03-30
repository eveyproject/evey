import { ScrollView, View, ActivityIndicator, RefreshControl } from "react-native";
import { useCallback, useMemo, useState } from "react";
import { router } from "expo-router";
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
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
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
        title="Continue Watching"
        items={resume.data?.items ?? []}
        onItemPress={handleItemPress}
        watchPercents={watchPercents}
      />
      <MediaRow
        title="Next Up"
        items={nextUp.data?.items ?? []}
        onItemPress={handleItemPress}
      />
      <MediaRow
        title="Recently Added"
        items={recentlyAdded.data?.items ?? []}
        onItemPress={handleItemPress}
      />
    </ScrollView>
  );
}
