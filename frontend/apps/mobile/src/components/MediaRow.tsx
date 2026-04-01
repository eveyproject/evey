import { useCallback, useMemo } from "react";
import { View, Text, FlatList, StyleSheet, Dimensions } from "react-native";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { MediaCard } from "./MediaCard";

const CARD_WIDTH = Dimensions.get("window").width * 0.3;
const CARD_TOTAL_WIDTH = CARD_WIDTH + 8; // card width + marginRight (theme.spacing.sm ~8)

interface MediaRowProps {
  title: string;
  items: V1MediaItem[];
  onItemPress?: (item: V1MediaItem) => void;
  watchPercents?: Record<string, number>;
}

const getItemLayout = (_: unknown, index: number) => ({
  length: CARD_TOTAL_WIDTH,
  offset: CARD_TOTAL_WIDTH * index,
  index,
});

export function MediaRow({ title, items, onItemPress, watchPercents }: MediaRowProps) {
  const theme = useTheme();

  const titleStyle = useMemo(() => ({
    color: theme.colors.text,
    marginBottom: theme.spacing.sm,
    paddingHorizontal: theme.spacing.md,
  }), [theme.colors.text, theme.spacing.sm, theme.spacing.md]);

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
      />
    ),
    [onItemPress, watchPercents],
  );

  if (!items.length) return null;

  return (
    <View style={{ marginBottom: theme.spacing.lg }}>
      <Text style={[styles.rowTitle, titleStyle]}>
        {title}
      </Text>
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
}

function keyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

const styles = StyleSheet.create({
  rowTitle: { fontSize: 18, fontWeight: "600" },
});
