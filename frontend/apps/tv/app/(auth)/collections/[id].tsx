import { useCallback } from "react";
import { View, Text, FlatList, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useCollection, type CollectionItem } from "../../../src/api/collections";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { ScreenState } from "../../../src/components/ScreenState";
import { artworkUrl } from "../../../src/lib/artwork";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";

const NUM_COLUMNS = 5;

function tvColKeyExtractor(item: CollectionItem) {
  return item.id;
}

function TVColSeparator() {
  return <View style={{ height: 16 }} />;
}

export default function CollectionDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const { data, isLoading, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useCollection(id ?? "");
  const col = data?.collection;
  const items = data?.items ?? [];

  const renderItem = useCallback(
    ({ item, index }: { item: CollectionItem; index: number }) => {
      const poster = serverUrl && item.id ? artworkUrl(serverUrl, item.id, "poster", 320) : undefined;
      return (
        <FocusableCard
          onPress={() => router.push(`/media/${item.id}`)}
          hasTVPreferredFocus={index === 0}
          style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
          accessibilityLabel={item.title}
        >
          <View style={[styles.poster, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm, overflow: "hidden" }]}>
            {poster ? (
              <Image
                source={{ uri: poster, headers: authHeaders }}
                style={StyleSheet.absoluteFill}
                contentFit="cover"
                transition={200}
              />
            ) : (
              <Text style={{ color: theme.colors.textMuted, fontSize: 14 }}>{t("common.noPoster")}</Text>
            )}
          </View>
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
            {item.title}
          </Text>
          {item.year ? (
            <Text style={[styles.year, { color: theme.colors.textMuted }]}>{item.year}</Text>
          ) : null}
        </FocusableCard>
      );
    },
    [theme, router, t, serverUrl, authHeaders],
  );

  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{col?.name ?? t("media.collections.title")}</Text>
      {col?.overview ? (
        <Text style={[styles.overview, { color: theme.colors.textMuted }]} numberOfLines={3}>
          {col.overview}
        </Text>
      ) : null}
      <FlatList
        data={items}
        keyExtractor={tvColKeyExtractor}
        renderItem={renderItem}
        numColumns={NUM_COLUMNS}
        contentContainerStyle={{ padding: 24 }}
        ItemSeparatorComponent={TVColSeparator}
        initialNumToRender={10}
        maxToRenderPerBatch={15}
        windowSize={5}
        removeClippedSubviews={true}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
        }}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("common.noItems")}</Text>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 16 },
  overview: { fontSize: 18, paddingHorizontal: 24, marginTop: -8, marginBottom: 16 },
  card: { flex: 1, margin: 8, padding: 12, alignItems: "center" },
  poster: { width: 160, height: 240, justifyContent: "center", alignItems: "center" },
  title: { fontSize: 16, fontWeight: "600", marginTop: 8, textAlign: "center" },
  year: { fontSize: 14, marginTop: 2 },
});
