import { useCallback, useMemo } from "react";
import { View, Text, FlatList, Pressable, StyleSheet } from "react-native";
import { ROW_CARD_WIDTH } from "../lib/rowCardWidth";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import type { Collection } from "../api/collections";

const CARD_WIDTH = ROW_CARD_WIDTH;
const CARD_HEIGHT = CARD_WIDTH * 1.5;
const CARD_TOTAL_WIDTH = CARD_WIDTH + 8;

interface CollectionRowProps {
  title: string;
  collections: Collection[];
  seeAllHref?: string;
}

const getItemLayout = (_: unknown, index: number) => ({
  length: CARD_TOTAL_WIDTH,
  offset: CARD_TOTAL_WIDTH * index,
  index,
});

import React from "react";
import { FONT_SIZE } from "@evey/theme";

export const CollectionRow = React.memo(function CollectionRow({ title, collections, seeAllHref }: CollectionRowProps) {
  const theme = useTheme();
  const router = useRouter();
  const { t } = useTranslation();

  const titleStyle = useMemo(
    () => ({
      color: theme.colors.text,
      marginBottom: theme.spacing.sm,
      paddingHorizontal: theme.spacing.md,
    }),
    [theme.colors.text, theme.spacing.sm, theme.spacing.md],
  );

  const contentStyle = useMemo(
    () => ({ paddingHorizontal: theme.spacing.md }),
    [theme.spacing.md],
  );

  const renderItem = useCallback(
    ({ item }: { item: Collection }) => {
      const initial = item.name?.[0]?.toUpperCase() ?? "?";
      const onPress = () => {
        if (item.rules && item.rules.length > 0) {
          router.push(`/collections/smart/${item.id}`);
        } else {
          router.push(`/collections/${item.id}`);
        }
      };

      return (
        <Pressable
          onPress={onPress}
          style={{ width: CARD_WIDTH, marginRight: theme.spacing.sm }}
          accessible
          accessibilityLabel={item.name}
          accessibilityRole="button"
        >
          <View
            style={{
              width: CARD_WIDTH,
              height: CARD_HEIGHT,
              borderRadius: theme.radii.md,
              backgroundColor: theme.colors.surface,
              overflow: "hidden",
              justifyContent: "center",
              alignItems: "center",
            }}
          >
            {item.posterUrl ? (
              <Image
                source={{ uri: item.posterUrl }}
                style={{ width: CARD_WIDTH, height: CARD_HEIGHT }}
                contentFit="cover"
                transition={200}
              />
            ) : (
              <Text
                style={{
                  color: theme.colors.textMuted,
                  fontSize: FONT_SIZE["3xl"],
                  fontWeight: "700",
                }}
              >
                {initial}
              </Text>
            )}
          </View>
          <Text
            numberOfLines={1}
            style={{ color: theme.colors.text, fontSize: FONT_SIZE.xs, marginTop: 4 }}
          >
            {item.name}
          </Text>
        </Pressable>
      );
    },
    [theme, router],
  );

  if (!collections.length) return null;

  return (
    <View style={{ marginBottom: theme.spacing.lg }}>
      <View style={styles.rowHeader}>
        <Text style={[styles.rowTitle, titleStyle]}>{title}</Text>
        {seeAllHref && (
          <Pressable onPress={() => router.push(seeAllHref as never)} style={{ paddingHorizontal: theme.spacing.md, marginBottom: theme.spacing.sm }} accessibilityRole="button" accessibilityLabel={t("common.seeAll", "See all")}>
            <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.xs }}>{t("common.seeAll", "See all")}</Text>
          </Pressable>
        )}
      </View>
      <FlatList
        horizontal
        data={collections}
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

function keyExtractor(item: Collection) {
  return item.id;
}

const styles = StyleSheet.create({
  rowHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  rowTitle: { fontSize: FONT_SIZE.lg, fontWeight: "600" },
});
