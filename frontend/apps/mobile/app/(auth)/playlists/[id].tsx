import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaylistDetail, type PlaylistItem } from "../../../src/api/playlists";

function PlaylistItemRow({
  item,
  theme,
  onPress,
}: {
  item: PlaylistItem;
  theme: ReturnType<typeof useTheme>;
  onPress: () => void;
}) {
  const typeLabel = item.type?.replace("MEDIA_ITEM_TYPE_", "").toLowerCase();

  return (
    <TouchableOpacity
      style={[styles.row, { borderBottomColor: theme.colors.border }]}
      activeOpacity={0.7}
      onPress={onPress}
    >
      <Text style={[styles.order, { color: theme.colors.accent }]}>
        {item.sort_order != null ? item.sort_order + 1 : "-"}
      </Text>
      <View style={{ flex: 1 }}>
        <Text style={[styles.itemTitle, { color: theme.colors.text }]} numberOfLines={1}>
          {item.title ?? item.media_item_id}
        </Text>
        <View style={{ flexDirection: "row", gap: 8, marginTop: 2 }}>
          {typeLabel ? (
            <Text style={[styles.badge, { color: theme.colors.textMuted }]}>{typeLabel}</Text>
          ) : null}
          {item.year ? (
            <Text style={[styles.badge, { color: theme.colors.textMuted }]}>{item.year}</Text>
          ) : null}
        </View>
      </View>
      <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} />
    </TouchableOpacity>
  );
}

export default function PlaylistDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const router = useRouter();

  const { data: playlist, isLoading, isError } = usePlaylistDetail(id ?? "");

  if (isLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Stack.Screen
          options={{
            headerShown: true,
            title: "Playlist",
            headerStyle: { backgroundColor: theme.colors.surface },
            headerTintColor: theme.colors.text,
          }}
        />
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (isError || !playlist) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Stack.Screen
          options={{
            headerShown: true,
            title: "Playlist",
            headerStyle: { backgroundColor: theme.colors.surface },
            headerTintColor: theme.colors.text,
          }}
        />
        <Text style={{ color: theme.colors.textMuted }}>Playlist not found</Text>
      </View>
    );
  }

  const items = playlist.items ?? [];

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: playlist.name,
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      {playlist.description ? (
        <Text style={[styles.description, { color: theme.colors.textMuted }]}>
          {playlist.description}
        </Text>
      ) : null}
      <FlatList
        data={items}
        keyExtractor={(item) => item.media_item_id}
        renderItem={({ item }) => (
          <PlaylistItemRow
            item={item}
            theme={theme}
            onPress={() => router.push(`/media/${item.media_item_id}`)}
          />
        )}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted }}>No items in this playlist</Text>
          </View>
        }
        ListFooterComponent={
          items.length > 0 ? (
            <Text style={[styles.footer, { color: theme.colors.textMuted }]}>
              {items.length} {items.length === 1 ? "item" : "items"}
            </Text>
          ) : null
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  description: { fontSize: 13, paddingHorizontal: 16, paddingTop: 12, paddingBottom: 8 },
  row: { flexDirection: "row", alignItems: "center", paddingHorizontal: 16, paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth },
  order: { width: 32, fontSize: 14, fontWeight: "600", fontVariant: ["tabular-nums"] },
  itemTitle: { fontSize: 15, fontWeight: "500" },
  badge: { fontSize: 11 },
  footer: { fontSize: 12, textAlign: "center", paddingVertical: 16 },
});
