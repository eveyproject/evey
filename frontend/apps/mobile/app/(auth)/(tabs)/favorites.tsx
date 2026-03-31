import { useCallback } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  ActivityIndicator,
  Dimensions,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import { Skeleton } from "../../../src/components/Skeleton";
import { useRouter } from "expo-router";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useFavoritesList, type FavoriteItem } from "../../../src/hooks/useFavorite";

const NUM_COLUMNS = 3;
const SPACING = 8;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH = (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;

export default function FavoritesScreen() {
  const theme = useTheme();
  const router = useRouter();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();
  const { data: favorites, isLoading, refetch } = useFavoritesList(200);

  const renderItem = useCallback(
    ({ item }: { item: FavoriteItem }) => {
      const posterUri = serverUrl
        ? `${serverUrl}/api/v1/media/${item.mediaItemId}/artwork/primary`
        : undefined;
      return (
        <TouchableOpacity
          style={[styles.gridItem, { width: ITEM_WIDTH, marginLeft: SPACING, marginBottom: SPACING }]}
          activeOpacity={0.7}
          onPress={() => router.push(`/media/${item.mediaItemId}`)}
        >
          {posterUri ? (
            <Image
              source={{ uri: posterUri, headers: authHeaders }}
              style={{ width: ITEM_WIDTH, height: POSTER_HEIGHT, borderRadius: theme.radii.sm }}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View
              style={{
                width: ITEM_WIDTH,
                height: POSTER_HEIGHT,
                borderRadius: theme.radii.sm,
                backgroundColor: theme.colors.surfaceRaised,
                justifyContent: "center",
                alignItems: "center",
              }}
            >
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>No poster</Text>
            </View>
          )}
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
            {item.title ?? "Untitled"}
          </Text>
          {item.year != null && (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, textAlign: "center" }}>
              {item.year}
            </Text>
          )}
        </TouchableOpacity>
      );
    },
    [theme, router, authHeaders, serverUrl],
  );

  if (isLoading) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl }]}>
          Favorites
        </Text>
        <View style={{ flexDirection: "row", flexWrap: "wrap", paddingTop: SPACING }}>
          {Array.from({ length: 9 }).map((_, i) => (
            <View key={i} style={{ marginLeft: SPACING, marginBottom: SPACING, alignItems: "center", width: ITEM_WIDTH }}>
              <Skeleton width={ITEM_WIDTH} height={POSTER_HEIGHT} borderRadius={theme.radii.sm} />
              <Skeleton width={ITEM_WIDTH * 0.7} height={12} borderRadius={4} style={{ marginTop: 6 }} />
            </View>
          ))}
        </View>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl }]}>
        Favorites
      </Text>
      <FlatList
        data={favorites ?? []}
        keyExtractor={(item) => item.mediaItemId}
        renderItem={renderItem}
        numColumns={NUM_COLUMNS}
        contentContainerStyle={{ paddingTop: SPACING }}
        refreshing={false}
        onRefresh={refetch}
        ListEmptyComponent={
          <View style={styles.empty}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>No favorites yet</Text>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  header: { fontSize: 28, fontWeight: "700", paddingHorizontal: 16 },
  gridItem: { alignItems: "center" },
  title: { fontSize: 12, fontWeight: "500", marginTop: 4, textAlign: "center" },
  empty: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 80 },
});
