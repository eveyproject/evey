import { memo, useCallback, useMemo } from "react";
import { View, Text, FlatList, StyleSheet, Pressable } from "react-native";
import { ROW_CARD_WIDTH } from "../lib/rowCardWidth";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useWatchedMap } from "../hooks/useWatchedMap";
import { MediaCard } from "./MediaCard";
import { RowTitle } from "./RowTitle";
import { FONT_SIZE } from "@evey/theme";

const CARD_WIDTH = ROW_CARD_WIDTH;
const CARD_TOTAL_WIDTH = CARD_WIDTH + 8; // card width + marginRight (theme.spacing.sm ~8)

interface MediaRowProps {
  title: string;
  seeAllHref?: string;
  items: V1MediaItem[];
  onItemPress?: (item: V1MediaItem) => void;
  watchPercents?: Record<string, number>;
}

const getItemLayout = (_: unknown, index: number) => ({
  length: CARD_TOTAL_WIDTH,
  offset: CARD_TOTAL_WIDTH * index,
  index,
});

export const MediaRow = memo(function MediaRow({ title, seeAllHref, items, onItemPress, watchPercents }: MediaRowProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const watchedMap = useWatchedMap();

  const titleStyle = useMemo(() => ({ paddingHorizontal: theme.spacing.md }), [theme.spacing.md]);

  const contentStyle = useMemo(
    () => ({ paddingHorizontal: theme.spacing.md }),
    [theme.spacing.md],
  );

  const renderItem = useCallback(
    ({ item }: { item: V1MediaItem }) => (
      <MediaCard
        item={item}
        onPress={() => onItemPress?.(item)}
        watchPercent={watchPercents?.[item.id ?? ""]}
        watched={item.id ? watchedMap.get(item.id) === true : false}
      />
    ),
    [onItemPress, watchPercents, watchedMap],
  );

  if (!items.length) return null;

  return (
    <View style={{ marginBottom: theme.spacing.lg }}>
      <View style={[styles.rowHeader, { marginBottom: theme.spacing.sm }]}>
        <RowTitle title={title} href={seeAllHref} style={titleStyle} />
        {seeAllHref && (
          <Pressable onPress={() => router.push(seeAllHref as never)} style={{ paddingHorizontal: theme.spacing.md }} accessible={true} accessibilityLabel={t("common.seeAll", "See all")} accessibilityRole="button">
            <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.xs }}>{t("common.seeAll", "See all")}</Text>
          </Pressable>
        )}
      </View>
      <FlatList
        horizontal
        data={items}
        keyExtractor={keyExtractor}
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={contentStyle}
        renderItem={renderItem}
        getItemLayout={getItemLayout}
        initialNumToRender={4}
        maxToRenderPerBatch={8}
        windowSize={5}
      />
    </View>
  );
});

function keyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

const styles = StyleSheet.create({
  rowHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
});
