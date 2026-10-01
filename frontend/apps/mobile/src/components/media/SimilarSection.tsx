import { memo, useCallback, useMemo } from "react";
import { View, Text, FlatList, Dimensions } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useFeedServiceSimilarItems, type V1MediaItem } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";
import { MediaCard } from "../MediaCard";

export const SimilarSection = memo(function SimilarSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const screenW = Dimensions.get("window").width;
  const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
  // The server's list or nothing: a same-genre fallback filled the row with
  // titles sharing one catch-all genre, which is not "similar" (2026-10-01).
  const cards: V1MediaItem[] = useMemo(
    () =>
      (similarData?.items ?? [])
        .filter((fi) => !!fi.mediaItemId)
        .map((fi) => ({ id: fi.mediaItemId!, title: fi.title })) as V1MediaItem[],
    [similarData?.items],
  );

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
