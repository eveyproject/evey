import { useState, useCallback } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  Pressable,
  ActivityIndicator,
  StyleSheet,
  Alert,
} from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaylistDetail, useDeletePlaylist, useRemoveFromPlaylist, useReorderPlaylistItems, useUpdatePlaylist, type PlaylistItem } from "../../../src/api/playlists";
import { SmartRulesEditor, type SmartRule } from "../../../src/components/SmartRulesEditor";

function PlaylistItemRow({
  item,
  index,
  total,
  isManual,
  theme,
  onPress,
  onMoveUp,
  onMoveDown,
  onRemove,
}: {
  item: PlaylistItem;
  index: number;
  total: number;
  isManual: boolean;
  theme: ReturnType<typeof useTheme>;
  onPress: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onRemove: () => void;
}) {
  const typeLabel = item.type?.replace("MEDIA_ITEM_TYPE_", "").toLowerCase();

  return (
    <TouchableOpacity
      style={[styles.row, { borderBottomColor: theme.colors.border }]}
      activeOpacity={0.7}
      onPress={onPress}
      accessible={true}
      accessibilityLabel={item.title ?? item.media_item_id}
      accessibilityRole="button"
    >
      {isManual && (
        <View style={{ alignItems: "center", marginRight: 4, gap: 2 }}>
          <Pressable onPress={onMoveUp} disabled={index === 0} hitSlop={6} accessible={true} accessibilityLabel="Move up" accessibilityRole="button">
            <Ionicons name="chevron-up" size={16} color={index === 0 ? theme.colors.border : theme.colors.textMuted} />
          </Pressable>
          <Text style={[styles.order, { color: theme.colors.accent }]}>
            {item.sort_order != null ? item.sort_order + 1 : index + 1}
          </Text>
          <Pressable onPress={onMoveDown} disabled={index === total - 1} hitSlop={6} accessible={true} accessibilityLabel="Move down" accessibilityRole="button">
            <Ionicons name="chevron-down" size={16} color={index === total - 1 ? theme.colors.border : theme.colors.textMuted} />
          </Pressable>
        </View>
      )}
      {!isManual && (
        <Text style={[styles.order, { color: theme.colors.accent }]}>
          {item.sort_order != null ? item.sort_order + 1 : "-"}
        </Text>
      )}
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
      {isManual && (
        <Pressable onPress={onRemove} hitSlop={8} style={{ padding: 4 }} accessible={true} accessibilityLabel="Remove" accessibilityRole="button">
          <Ionicons name="close-circle-outline" size={18} color={theme.colors.textMuted} />
        </Pressable>
      )}
      <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} />
    </TouchableOpacity>
  );
}

export default function PlaylistDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const deleteMutation = useDeletePlaylist();
  const removeMutation = useRemoveFromPlaylist();
  const reorderMutation = useReorderPlaylistItems();

  const { data: playlist, isLoading, isError } = usePlaylistDetail(id ?? "");
  const updatePlaylist = useUpdatePlaylist();
  const isManual = playlist?.type === "manual";
  const isSmart = playlist?.type?.includes("smart") ?? false;
  const [showRulesEditor, setShowRulesEditor] = useState(false);

  const confirmDelete = () => {
    Alert.alert(
      t("common.delete"),
      t("media.playlist.confirmDelete", { name: playlist?.name }),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("common.delete"),
          style: "destructive",
          onPress: async () => {
            try {
              await deleteMutation.mutateAsync(id ?? "");
              router.back();
            } catch {
              Alert.alert(t("common.error"), t("media.playlist.deleteError"));
            }
          },
        },
      ],
    );
  };

  const handleMove = useCallback(async (fromIdx: number, dir: -1 | 1) => {
    if (!playlist?.items || !id) return;
    const items = [...playlist.items];
    const toIdx = fromIdx + dir;
    if (toIdx < 0 || toIdx >= items.length) return;
    [items[fromIdx], items[toIdx]] = [items[toIdx], items[fromIdx]];
    const reordered = items.map((it, i) => ({ mediaItemId: it.media_item_id, sortOrder: i }));
    // mutate, not mutateAsync: the hook's own onError already reports the
    // failure, and an awaited rejection nobody catches is an unhandled promise.
    reorderMutation.mutate({ playlistId: id, items: reordered });
  }, [playlist, id, reorderMutation]);

  const handleRemove = useCallback((mediaItemId: string) => {
    if (!id) return;
    Alert.alert(t("common.remove") || "Remove", t("media.playlist.confirmRemoveItem") || "Remove from playlist?", [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("common.remove") || "Remove", style: "destructive", onPress: () => removeMutation.mutate({ playlistId: id, mediaItemId }) },
    ]);
  }, [id, removeMutation, t]);

  const headerRight = () => (
    <TouchableOpacity onPress={confirmDelete} style={{ marginRight: 8 }} hitSlop={8} accessible={true} accessibilityLabel={t("common.delete")} accessibilityRole="button">
      <Ionicons name="trash-outline" size={22} color={theme.colors.error} />
    </TouchableOpacity>
  );

  if (isLoading) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Stack.Screen
          options={{
            headerShown: true,
            title: t("common.playlists"),
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
            title: t("common.playlists"),
            headerStyle: { backgroundColor: theme.colors.surface },
            headerTintColor: theme.colors.text,
          }}
        />
        <Text style={{ color: theme.colors.textMuted }}>{t("media.playlist.notFound")}</Text>
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
          headerRight,
        }}
      />
      {playlist.description ? (
        <Text style={[styles.description, { color: theme.colors.textMuted }]}>
          {playlist.description}
        </Text>
      ) : null}
      {isSmart && (
        <Pressable
          onPress={() => setShowRulesEditor(true)}
          style={{ flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: 16, paddingVertical: 8 }}
          accessible={true}
          accessibilityLabel={t("media.playlist.editRules") || "Edit Rules"}
          accessibilityRole="button"
        >
          <Ionicons name="options-outline" size={16} color={theme.colors.accent} />
          <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
            {t("media.playlist.editRules") || "Edit Rules"} ({playlist.rules?.length ?? 0})
          </Text>
        </Pressable>
      )}
      <FlatList
        data={items}
        keyExtractor={(item) => item.media_item_id}
        renderItem={({ item, index }) => (
          <PlaylistItemRow
            item={item}
            index={index}
            total={items.length}
            isManual={isManual}
            theme={theme}
            onPress={() => router.push(`/media/${item.media_item_id}`)}
            onMoveUp={() => handleMove(index, -1)}
            onMoveDown={() => handleMove(index, 1)}
            onRemove={() => handleRemove(item.media_item_id)}
          />
        )}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted }}>{t("common.noItems")}</Text>
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
      {isSmart && (
        <SmartRulesEditor
          visible={showRulesEditor}
          onClose={() => setShowRulesEditor(false)}
          rules={(playlist.rules ?? []).map((r) => ({ field: r.field as SmartRule["field"], operator: (r.operator || "eq") as SmartRule["operator"], value: r.value }))}
          onSave={(newRules) => {
            if (!id) return;
            // The sheet closes right after this call regardless (SmartRulesEditor
            // does not await onSave), so the failure has to speak for itself:
            // useUpdatePlaylist's own onError toast does that.
            updatePlaylist.mutate({ id, rules: newRules });
          }}
        />
      )}
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
