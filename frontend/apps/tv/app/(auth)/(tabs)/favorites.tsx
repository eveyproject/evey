import { useCallback } from "react";
import { View, Text, FlatList, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useFavoritesList, type FavoriteItem } from "../../../src/hooks/useFavorite";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { ScreenState } from "../../../src/components/ScreenState";
import { CARD_WIDTH_GRID, POSTER_RATIO, LABEL_BLOCK_HEIGHT, GRID_GAP } from "../../../src/lib/tvLayout";

const NUM_COLUMNS = 5;
const CARD_WIDTH = CARD_WIDTH_GRID;
const POSTER_HEIGHT = CARD_WIDTH * POSTER_RATIO;
const ROW_HEIGHT = POSTER_HEIGHT + LABEL_BLOCK_HEIGHT + GRID_GAP; // poster + text + gap

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
  const { data: favorites, isLoading, isError, refetch } = useFavoritesList();

  const renderItem = useCallback(
    ({ item, index }: { item: FavoriteItem; index: number }) => (
      <FocusableCard
        onPress={() => router.push(`/media/${item.mediaItemId}`)}
        hasTVPreferredFocus={index === 0}
        style={styles.gridItem}
        accessibilityLabel={item.title ?? t("common.untitled", "Untitled")}
      >
        <View style={[styles.poster, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 17 }}>?</Text>
        </View>
        <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
          {item.title ?? t("common.untitled", "Untitled")}
        </Text>
        {item.year != null && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, textAlign: "center" }}>
            {item.year}
          </Text>
        )}
      </FocusableCard>
    ),
    [theme, router, t],
  );

  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("media.favorites.title")}</Text>
      <FlatList
        data={favorites ?? []}
        keyExtractor={tvFavKeyExtractor}
        renderItem={renderItem}
        numColumns={NUM_COLUMNS}
        contentContainerStyle={{ padding: 24 }}
        columnWrapperStyle={{ gap: 12, marginBottom: 16 }}
        getItemLayout={tvFavGetItemLayout}
        initialNumToRender={10}
        maxToRenderPerBatch={15}
        windowSize={5}
        removeClippedSubviews={true}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 20, marginBottom: 16 }}>{t("media.favorites.empty")}</Text>
            {/* A sentence and nothing else is a dead end: the viewer has to
                guess that the way out is the rail. Same affordance the empty
                collections and playlists screens already offer. */}
            <FocusableCard
              onPress={() => router.push("/(auth)/(tabs)/library")}
              style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
              accessibilityLabel={t("media.library.libraries")}
            >
              <Text style={{ color: theme.colors.accent, fontSize: 17, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
            </FocusableCard>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 30, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32 },
  emptyAction: { paddingHorizontal: 24, paddingVertical: 10 },
  gridItem: { width: CARD_WIDTH, alignItems: "center" },
  poster: {
    width: CARD_WIDTH,
    height: POSTER_HEIGHT,
    justifyContent: "center",
    alignItems: "center",
  },
  title: { fontSize: 15, fontWeight: "500", marginTop: 8, textAlign: "center" },
});
