import { useState } from "react";
import { View, Text, TouchableOpacity, Pressable, FlatList, ActivityIndicator, Modal, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { collectionServiceAddCollectionItem } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { useCollections } from "../../api/collections";
import { usePlaylistList, useAddToPlaylist } from "../../api/playlists";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

// Extracted out of library/[libraryId].tsx so the favorites screen's bulk
// action bar can add a full selection to a playlist/collection too, instead
// of only offering watched/unwatched (the parity gap with web's BulkActionBar).

// The shared AddToPlaylistSheet only accepts a single mediaItemId (built for
// the per-item long-press menu), so bulk add needs its own sheet - mirrors
// BulkAddToCollectionSheet just below and web's BulkPlaylistModal
// (library/BulkPlaylistModal.tsx), both of which add the FULL selection via
// Promise.all instead of a single id (the bug this fixes).
export function BulkAddToPlaylistSheet({
  visible,
  mediaItemIds,
  onClose,
}: {
  visible: boolean;
  mediaItemIds: string[];
  onClose: () => void;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data: playlists, isLoading } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");
  const [pendingId, setPendingId] = useState<string | null>(null);

  async function handleAdd(playlistId: string, name: string) {
    setPendingId(playlistId);
    try {
      await Promise.all(mediaItemIds.map((id) => addTo.mutateAsync({ playlistId, mediaItemId: id })));
      toast.success(t("media.library.itemsAddedToPlaylist", { count: mediaItemIds.length, name }));
      onClose();
    } catch {
      toast.error(t("media.library.failedAddPlaylist"));
    } finally {
      setPendingId(null);
    }
  }

  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={onClose}>
      <Pressable style={[collectionSheetStyles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={onClose}>
        <Pressable
          style={[collectionSheetStyles.sheet, { backgroundColor: theme.colors.surface, borderTopLeftRadius: theme.radii.lg, borderTopRightRadius: theme.radii.lg }]}
          onPress={(e) => e.stopPropagation()}
        >
          <View style={collectionSheetStyles.handle}>
            <View style={[collectionSheetStyles.handleBar, { backgroundColor: theme.colors.textMuted }]} />
          </View>
          <Text style={[collectionSheetStyles.title, { color: theme.colors.text }]}>
            {t("media.library.addToPlaylist")}
          </Text>

          {isLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginTop: 24 }} />
          ) : manualPlaylists.length === 0 ? (
            <Text style={[collectionSheetStyles.empty, { color: theme.colors.textMuted }]}>
              {t("media.library.noPlaylists")}
            </Text>
          ) : (
            <FlatList
              data={manualPlaylists}
              keyExtractor={(item) => item.id}
              style={{ maxHeight: 320 }}
              renderItem={({ item }) => (
                <TouchableOpacity
                  style={[collectionSheetStyles.row, { borderBottomColor: theme.colors.border }]}
                  activeOpacity={0.7}
                  onPress={() => void handleAdd(item.id, item.name)}
                  disabled={pendingId !== null}
                >
                  <Ionicons name="list" size={ICON_SIZE.lg} color={theme.colors.accent} />
                  <Text style={[collectionSheetStyles.collectionName, { color: theme.colors.text }]} numberOfLines={1}>
                    {item.name}
                  </Text>
                  {pendingId === item.id ? (
                    <ActivityIndicator size="small" color={theme.colors.accent} />
                  ) : (
                    <Ionicons name="add-circle-outline" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
                  )}
                </TouchableOpacity>
              )}
            />
          )}

          <TouchableOpacity
            style={[collectionSheetStyles.closeBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
            onPress={onClose}
          >
            <Text style={[collectionSheetStyles.closeBtnText, { color: theme.colors.text }]}>{t("common.close", "Close")}</Text>
          </TouchableOpacity>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

// Mobile has no shared "add to collection" sheet (unlike AddToPlaylistSheet),
// so this mirrors that component's structure locally rather than growing a
// new shared component outside this screen's whitelist. Reuses the existing
// useCollections list hook and the generated collectionServiceAddCollectionItem
// mutation - same pieces web's AddToCollectionModal is built from.
export function BulkAddToCollectionSheet({
  visible,
  mediaItemIds,
  onClose,
}: {
  visible: boolean;
  mediaItemIds: string[];
  onClose: () => void;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data, isLoading } = useCollections();
  const collections = data?.collections ?? [];
  const [pendingId, setPendingId] = useState<string | null>(null);

  async function handleAdd(collectionId: string, name: string) {
    setPendingId(collectionId);
    try {
      await Promise.all(
        mediaItemIds.map((id) => collectionServiceAddCollectionItem(collectionId, { mediaItemId: id, sortOrder: 0 })),
      );
      toast.success(t("media.collections.itemsAdded", { count: mediaItemIds.length, name }));
      onClose();
    } catch {
      toast.error(t("media.collections.addFailed"));
    } finally {
      setPendingId(null);
    }
  }

  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={onClose}>
      <Pressable style={[collectionSheetStyles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={onClose}>
        <Pressable
          style={[collectionSheetStyles.sheet, { backgroundColor: theme.colors.surface, borderTopLeftRadius: theme.radii.lg, borderTopRightRadius: theme.radii.lg }]}
          onPress={(e) => e.stopPropagation()}
        >
          <View style={collectionSheetStyles.handle}>
            <View style={[collectionSheetStyles.handleBar, { backgroundColor: theme.colors.textMuted }]} />
          </View>
          <Text style={[collectionSheetStyles.title, { color: theme.colors.text }]}>
            {t("media.contextMenu.addToCollection", "Add to Collection")}
          </Text>

          {isLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginTop: 24 }} />
          ) : collections.length === 0 ? (
            <Text style={[collectionSheetStyles.empty, { color: theme.colors.textMuted }]}>
              {t("media.collections.noCollectionsAvailable")}
            </Text>
          ) : (
            <FlatList
              data={collections}
              keyExtractor={(item) => item.id}
              style={{ maxHeight: 320 }}
              renderItem={({ item }) => (
                <TouchableOpacity
                  style={[collectionSheetStyles.row, { borderBottomColor: theme.colors.border }]}
                  activeOpacity={0.7}
                  onPress={() => void handleAdd(item.id, item.name)}
                  disabled={pendingId !== null}
                >
                  <Ionicons name="albums" size={ICON_SIZE.lg} color={theme.colors.accent} />
                  <Text style={[collectionSheetStyles.collectionName, { color: theme.colors.text }]} numberOfLines={1}>
                    {item.name}
                  </Text>
                  {pendingId === item.id ? (
                    <ActivityIndicator size="small" color={theme.colors.accent} />
                  ) : (
                    <Ionicons name="add-circle-outline" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
                  )}
                </TouchableOpacity>
              )}
            />
          )}

          <TouchableOpacity
            style={[collectionSheetStyles.closeBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
            onPress={onClose}
          >
            <Text style={[collectionSheetStyles.closeBtnText, { color: theme.colors.text }]}>{t("common.close", "Close")}</Text>
          </TouchableOpacity>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const collectionSheetStyles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "flex-end" },
  sheet: { paddingBottom: 34, paddingHorizontal: 16 },
  handle: { alignItems: "center", paddingVertical: 10 },
  handleBar: { width: 40, height: 4, borderRadius: 2, opacity: 0.4 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700", marginBottom: 12 },
  empty: { fontSize: FONT_SIZE.sm, textAlign: "center", marginTop: 24 },
  row: { flexDirection: "row", alignItems: "center", paddingVertical: 14, gap: 12, borderBottomWidth: StyleSheet.hairlineWidth },
  collectionName: { flex: 1, fontSize: FONT_SIZE.sm, fontWeight: "500" },
  closeBtn: { marginTop: 16, paddingVertical: 12, alignItems: "center" },
  closeBtnText: { fontSize: FONT_SIZE.sm, fontWeight: "600" },
});
