import { useCallback } from "react";
import { View, Text, FlatList, Pressable, StyleSheet, ActivityIndicator } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useFavoritesList, type FavoriteItem } from "../../../src/hooks/useFavorite";
import { FocusableCard } from "../../../src/components/FocusableCard";

const NUM_COLUMNS = 5;
const CARD_WIDTH = 200;
const POSTER_HEIGHT = CARD_WIDTH * 1.5;
const ROW_HEIGHT = POSTER_HEIGHT + 40 + 16; // poster + text + gap

function tvFavKeyExtractor(item: FavoriteItem) {
  return item.mediaItemId;
}

const tvFavGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * Math.floor(index / NUM_COLUMNS),
  index,
});

export default function FavoritesScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const { data: favorites, isLoading, isError, refetch } = useFavoritesList(200);

  const renderItem = useCallback(
    ({ item, index }: { item: FavoriteItem; index: number }) => (
      <FocusableCard
        onPress={() => router.push(`/media/${item.mediaItemId}`)}
        hasTVPreferredFocus={index === 0}
        style={styles.gridItem}
      >
        <View style={[styles.poster, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 18 }}>?</Text>
        </View>
        <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
          {item.title ?? "Untitled"}
        </Text>
        {item.year != null && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 14, textAlign: "center" }}>
            {item.year}
          </Text>
        )}
      </FocusableCard>
    ),
    [theme, router],
  );

  if (isError) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 12 }}>{t("common.loadError")}</Text>
        <Pressable onPress={() => refetch()}>
          <Text style={{ color: theme.colors.accent, fontSize: 20 }}>{t("common.retry")}</Text>
        </Pressable>
      </View>
    );
  }

  if (isLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>Favorites</Text>
      <FlatList
        data={favorites ?? []}
        keyExtractor={tvFavKeyExtractor}
        renderItem={renderItem}
        numColumns={NUM_COLUMNS}
        contentContainerStyle={{ padding: 24 }}
        columnWrapperStyle={{ gap: 16, marginBottom: 16 }}
        getItemLayout={tvFavGetItemLayout}
        initialNumToRender={10}
        maxToRenderPerBatch={15}
        windowSize={5}
        removeClippedSubviews={true}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("media.favorites.empty")}</Text>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32 },
  gridItem: { width: CARD_WIDTH, alignItems: "center" },
  poster: {
    width: CARD_WIDTH,
    height: POSTER_HEIGHT,
    justifyContent: "center",
    alignItems: "center",
  },
  title: { fontSize: 16, fontWeight: "500", marginTop: 8, textAlign: "center" },
});
