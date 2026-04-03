import { useCallback, useMemo } from "react";
import { View, Text, FlatList, Pressable, StyleSheet, Dimensions } from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { useTheme } from "../providers/ThemeProvider";
import type { Collection } from "../api/collections";

const CARD_WIDTH = Dimensions.get("window").width * 0.35;
const CARD_HEIGHT = CARD_WIDTH * 1.5;
const CARD_TOTAL_WIDTH = CARD_WIDTH + 8;

interface CollectionRowProps {
  title: string;
  collections: Collection[];
}

const getItemLayout = (_: unknown, index: number) => ({
  length: CARD_TOTAL_WIDTH,
  offset: CARD_TOTAL_WIDTH * index,
  index,
});

export function CollectionRow({ title, collections }: CollectionRowProps) {
  const theme = useTheme();
  const router = useRouter();

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
      const initial = item.name[0]?.toUpperCase() ?? "?";
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
                  fontSize: 28,
                  fontWeight: "700",
                }}
              >
                {initial}
              </Text>
            )}
          </View>
          <Text
            numberOfLines={1}
            style={{ color: theme.colors.text, fontSize: 13, marginTop: 4 }}
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
      <Text style={[styles.rowTitle, titleStyle]}>{title}</Text>
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
}

function keyExtractor(item: Collection) {
  return item.id;
}

const styles = StyleSheet.create({
  rowTitle: { fontSize: 18, fontWeight: "600" },
});
