import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  StyleSheet,
  Dimensions,
} from "react-native";
import { Image } from "expo-image";
import { Stack, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { Skeleton } from "../../../src/components/Skeleton";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useCollections, type Collection } from "../../../src/api/collections";

const NUM_COLUMNS = 2;
const SPACING = 12;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH =
  (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;

function CollectionCard({
  col,
  theme,
  onPress,
}: {
  col: Collection;
  theme: ReturnType<typeof useTheme>;
  onPress: () => void;
}) {
  const initial = col.name[0]?.toUpperCase() ?? "?";

  return (
    <TouchableOpacity
      style={[styles.card, { width: ITEM_WIDTH, marginLeft: SPACING, marginBottom: SPACING }]}
      activeOpacity={0.7}
      onPress={onPress}
    >
      {col.posterUrl ? (
        <Image
          source={{ uri: col.posterUrl }}
          style={[
            styles.poster,
            {
              width: ITEM_WIDTH,
              height: POSTER_HEIGHT,
              borderRadius: theme.radii.sm,
            },
          ]}
          contentFit="cover"
          transition={200}
        />
      ) : (
        <View
          style={[
            styles.placeholder,
            {
              width: ITEM_WIDTH,
              height: POSTER_HEIGHT,
              borderRadius: theme.radii.sm,
              backgroundColor: theme.colors.surfaceRaised,
            },
          ]}
        >
          <Text style={{ color: theme.colors.textMuted, fontSize: 32, fontWeight: "700" }}>
            {initial}
          </Text>
        </View>
      )}
      <Text
        style={[styles.name, { color: theme.colors.text }]}
        numberOfLines={2}
      >
        {col.name}
      </Text>
    </TouchableOpacity>
  );
}

export default function CollectionsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const { data, isLoading, refetch } = useCollections();

  const collections = data?.collections ?? [];

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: "Collections",
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      {isLoading ? (
        <View style={styles.skeletonGrid}>
          {Array.from({ length: 6 }).map((_, i) => (
            <View key={i} style={{ marginLeft: SPACING, marginBottom: SPACING }}>
              <Skeleton width={ITEM_WIDTH} height={POSTER_HEIGHT} borderRadius={theme.radii.sm} />
              <Skeleton width={ITEM_WIDTH * 0.7} height={12} borderRadius={4} style={{ marginTop: 6 }} />
            </View>
          ))}
        </View>
      ) : (
        <FlatList
          data={collections}
          keyExtractor={(item) => item.id}
          numColumns={NUM_COLUMNS}
          contentContainerStyle={{ paddingTop: SPACING }}
          refreshing={false}
          onRefresh={refetch}
          renderItem={({ item }) => (
            <CollectionCard
              col={item}
              theme={theme}
              onPress={() => {
                if (item.rules && item.rules.length > 0) {
                  router.push(`/collections/smart/${item.id}`);
                } else {
                  router.push(`/collections/${item.id}`);
                }
              }}
            />
          )}
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted }}>{t("common.noCollections")}</Text>
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
  card: { alignItems: "center" },
  poster: { width: "100%" },
  placeholder: { justifyContent: "center", alignItems: "center" },
  name: { fontSize: 13, fontWeight: "600", marginTop: 4, textAlign: "center" },
});
