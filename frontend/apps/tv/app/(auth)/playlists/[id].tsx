import { View, Text, FlatList, StyleSheet } from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { mediaTypeLabelKey } from "../../../src/lib/mediaTypeLabel";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaylistDetail, type PlaylistItem } from "../../../src/api/playlists";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { musicServiceGetTrack } from "@evey/api";
import { trackToQueueTrack, useMusicQueueStore } from "../../../src/stores/musicQueue";
import { ScreenState } from "../../../src/components/ScreenState";

export default function PlaylistDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  // A music track has no video page: /media/<track id> answers 404. It plays in
  // the music player, from its own metadata (the playlist carries only its id).
  const playTrack = (trackId: string) => {
    void musicServiceGetTrack(trackId)
      .then((res) => setQueue([trackToQueueTrack(res.track ?? { id: trackId })], 0))
      .catch(() => setQueue([trackToQueueTrack({ id: trackId })], 0));
  };

  const { data: playlist, isLoading, isError, refetch } = usePlaylistDetail(id ?? "");

  // isError (the request failed) and !playlist (it answered with nothing) used
  // to share one branch, so a network failure read as "playlist not found" -
  // the exact wrong-permanence a retry is meant to fix.
  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  }

  if (!playlist) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>{t("media.playlists.notFound")}</Text>
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
            onPress={() =>
              item.media_type === "music" ? playTrack(item.media_item_id) : router.push(`/media/${item.media_item_id}`)
            }
          />
        )}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>{t("common.noItems")}</Text>
          </View>
        }
        ListFooterComponent={
          items.length > 0 ? (
            <Text style={[styles.footer, { color: theme.colors.textMuted }]}>
              {t("common.item", { count: items.length })}
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
  const { t } = useTranslation();
  // Translated, never the raw enum: a viewer read "movie" in every language.
  const typeKey = mediaTypeLabelKey(item.type);
  const typeLabel = typeKey ? t(typeKey) : "";

  return (
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      style={[styles.row, { borderBottomColor: theme.colors.border }]}
      accessibilityLabel={item.title ?? item.media_item_id}
    >
      <Text style={[styles.order, { color: theme.colors.accent }]}>
        {item.sort_order != null ? item.sort_order + 1 : "-"}
      </Text>
      <View style={{ flex: 1 }}>
        <Text style={[styles.itemTitle, { color: theme.colors.text }]} numberOfLines={1}>
          {item.title ?? item.media_item_id}
        </Text>
        <View style={{ flexDirection: "row", gap: 10, marginTop: 4 }}>
          {typeLabel ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{typeLabel}</Text>
          ) : null}
          {item.year ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{item.year}</Text>
          ) : null}
        </View>
      </View>
      <Text style={{ color: theme.colors.textMuted, fontSize: 21 }}>{"\u203A"}</Text>
    </FocusableCard>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 30, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 8 },
  description: { fontSize: 17, paddingHorizontal: 24, paddingBottom: 16 },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 20,
    paddingVertical: 11,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  order: { width: 48, fontSize: 18, fontWeight: "600", fontVariant: ["tabular-nums"] },
  itemTitle: { fontSize: 18, fontWeight: "500" },
  footer: { fontSize: 15, textAlign: "center", paddingVertical: 16 },
});
