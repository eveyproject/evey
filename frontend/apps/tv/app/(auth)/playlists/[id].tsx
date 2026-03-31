import { View, Text, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaylistDetail, type PlaylistItem } from "../../../src/api/playlists";
import { FocusableCard } from "../../../src/components/FocusableCard";

export default function PlaylistDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const router = useRouter();

  const { data: playlist, isLoading, isError } = usePlaylistDetail(id ?? "");

  if (isLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (isError || !playlist) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>Playlist not found</Text>
      </View>
    );
  }

  const items = playlist.items ?? [];

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{playlist.name}</Text>
      {playlist.description ? (
        <Text style={[styles.description, { color: theme.colors.textMuted }]}>{playlist.description}</Text>
      ) : null}
      <FlatList
        data={items}
        keyExtractor={(item) => item.media_item_id}
        contentContainerStyle={{ paddingHorizontal: 24 }}
        ItemSeparatorComponent={() => <View style={{ height: 4 }} />}
        renderItem={({ item, index }) => (
          <PlaylistItemRow
            item={item}
            theme={theme}
            isFirst={index === 0}
            onPress={() => router.push(`/media/${item.media_item_id}`)}
          />
        )}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>No items in this playlist</Text>
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

function PlaylistItemRow({
  item,
  theme,
  isFirst,
  onPress,
}: {
  item: PlaylistItem;
  theme: ReturnType<typeof useTheme>;
  isFirst?: boolean;
  onPress: () => void;
}) {
  const typeLabel = item.type?.replace("MEDIA_ITEM_TYPE_", "").toLowerCase();

  return (
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      style={[styles.row, { borderBottomColor: theme.colors.border }]}
    >
      <Text style={[styles.order, { color: theme.colors.accent }]}>
        {item.sort_order != null ? item.sort_order + 1 : "-"}
      </Text>
      <View style={{ flex: 1 }}>
        <Text style={[styles.itemTitle, { color: theme.colors.text }]} numberOfLines={1}>
          {item.title ?? item.media_item_id}
        </Text>
        <View style={{ flexDirection: "row", gap: 12, marginTop: 4 }}>
          {typeLabel ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>{typeLabel}</Text>
          ) : null}
          {item.year ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>{item.year}</Text>
          ) : null}
        </View>
      </View>
      <Text style={{ color: theme.colors.textMuted, fontSize: 24 }}>{"\u203A"}</Text>
    </FocusableCard>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 8 },
  description: { fontSize: 18, paddingHorizontal: 24, paddingBottom: 16 },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 20,
    paddingVertical: 16,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  order: { width: 48, fontSize: 20, fontWeight: "600", fontVariant: ["tabular-nums"] },
  itemTitle: { fontSize: 20, fontWeight: "500" },
  footer: { fontSize: 16, textAlign: "center", paddingVertical: 24 },
});
