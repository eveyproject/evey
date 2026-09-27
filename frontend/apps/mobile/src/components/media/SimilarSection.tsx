import { memo, useCallback, useMemo } from "react";
import { View, Text, FlatList, Dimensions } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useFeedServiceSimilarItems, useLibraryServiceListMediaItems, type V1MediaItem } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";
import { MediaCard } from "../MediaCard";

export const SimilarSection = memo(function SimilarSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const screenW = Dimensions.get("window").width;
  const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
  const firstGenre = item.genres?.[0];
  const hasSimilar = (similarData?.items?.length ?? 0) > 0;
  const { data: byGenreData } = useLibraryServiceListMediaItems(
    { genre: firstGenre, type: item.type as never },
    { query: { enabled: !hasSimilar && !!firstGenre } } as never,
  );

  const cards: V1MediaItem[] = useMemo(() => {
    const fromFeed = (similarData?.items ?? [])
      .filter((fi: any) => !!fi.mediaItemId)
      .map((fi: any) => ({ id: fi.mediaItemId!, title: fi.title }));

    const fromGenre = (byGenreData?.items ?? [])
      .filter((m) => m.id && m.id !== item.id)
      .map((m) => ({ id: m.id!, title: m.title, year: m.year, type: m.type }));

    return (fromFeed.length ? fromFeed : fromGenre) as V1MediaItem[];
  }, [similarData?.items, byGenreData?.items, item.id]);

  const renderCard = useCallback(
    ({ item: c }: { item: V1MediaItem }) => (
      <MediaCard
        item={c}
        onPress={() => c.id && router.push(`/(auth)/media/${c.id}`)}
        width={screenW * 0.28}
      />
    ),
    [screenW],
  );

  if (!cards.length) return null;

  return (
    <View style={{ marginTop: theme.spacing.lg }}>
      <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
        {t("media.detail.similarTitles")}
      </Text>
      <FlatList
        horizontal
        data={cards}
        keyExtractor={similarKeyExtractor}
        renderItem={renderCard}
        showsHorizontalScrollIndicator={false}
      />
    </View>
  );
});

function similarKeyExtractor(c: V1MediaItem) {
  return c.id ?? "";
}
