import { useCallback } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  Dimensions,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { Skeleton } from "../../../src/components/Skeleton";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useCollection, type CollectionItem } from "../../../src/api/collections";
import { artworkUrl } from "../../../src/lib/artwork";

const NUM_COLUMNS = 3;
const SPACING = 8;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH = (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;

export default function CollectionDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const { data, isLoading } = useCollection(id ?? "");
  const col = data?.collection;
  const items = data?.items ?? [];

  const renderItem = useCallback(
    ({ item }: { item: CollectionItem }) => {
      const poster = serverUrl && item.id ? artworkUrl(serverUrl, item.id, "primary") : undefined;
      return (
        <TouchableOpacity
          style={[styles.gridItem, { width: ITEM_WIDTH, marginLeft: SPACING, marginBottom: SPACING }]}
          activeOpacity={0.7}
          onPress={() => router.push(`/media/${item.id}`)}
        >
          {poster ? (
            <Image
              source={{ uri: poster, headers: authHeaders }}
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
          <Text
            style={[styles.title, { color: theme.colors.text }]}
            numberOfLines={2}
          >
            {item.title}
          </Text>
        </TouchableOpacity>
      );
    },
    [theme, router, serverUrl, authHeaders],
  );

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: col?.name ?? "Collection",
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      {isLoading ? (
        <View style={styles.skeletonGrid}>
          {Array.from({ length: 9 }).map((_, i) => (
            <View key={i} style={{ marginLeft: SPACING, marginBottom: SPACING, alignItems: "center", width: ITEM_WIDTH }}>
              <Skeleton width={ITEM_WIDTH} height={POSTER_HEIGHT} borderRadius={theme.radii.sm} />
              <Skeleton width={ITEM_WIDTH * 0.7} height={12} borderRadius={4} style={{ marginTop: 6 }} />
            </View>
          ))}
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(item) => item.id}
          renderItem={renderItem}
          numColumns={NUM_COLUMNS}
          contentContainerStyle={{ paddingTop: SPACING }}
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted }}>No items in this collection</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 64 },
  skeletonGrid: { flexDirection: "row", flexWrap: "wrap", paddingTop: SPACING },
  gridItem: { alignItems: "center" },
  title: { fontSize: 12, fontWeight: "500", marginTop: 4, textAlign: "center" },
});
