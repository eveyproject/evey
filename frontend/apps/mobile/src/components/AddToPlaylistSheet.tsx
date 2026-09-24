import { useState } from "react";
import {
  View,
  Text,
  Modal,
  FlatList,
  TouchableOpacity,
  Pressable,
  TextInput,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { usePlaylistList, useAddToPlaylist, useCreatePlaylist } from "../api/playlists";
import { LoadFailed } from "./LoadFailed";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface Props {
  visible: boolean;
  mediaItemId: string;
  mediaType?: string;
  onClose: () => void;
}

export function AddToPlaylistSheet({ visible, mediaItemId, mediaType, onClose }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data: playlists, isLoading, isError, refetch } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const createPl = useCreatePlaylist();
  const [addedIds, setAddedIds] = useState<Set<string>>(new Set());
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");

  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");

  function handleAdd(playlistId: string) {
    addTo.mutate(
      { playlistId, mediaItemId, mediaType },
      {
        onSuccess: () => {
          setAddedIds((prev) => new Set(prev).add(playlistId));
          toast.success(t("media.playlist.addSuccess"));
        },
        onError: () => toast.error(t("media.playlist.addError")),
      },
    );
  }

  async function handleCreateAndAdd() {
    const name = newName.trim();
    if (!name) return;
    let created = false;
    try {
      const res = await createPl.mutateAsync({ name, type: "manual" });
      created = true;
      toast.success(t("media.playlist.createSuccess"));
      if (res.id) {
        await addTo.mutateAsync({ playlistId: res.id, mediaItemId, mediaType });
        setAddedIds((prev) => new Set(prev).add(res.id));
        toast.success(t("media.playlist.addSuccess"));
      }
      setCreating(false);
      setNewName("");
    } catch {
      toast.error(t(created ? "media.playlist.addError" : "media.playlist.createError"));
    }
  }

  function handleCloseAll() {
    setCreating(false);
    setNewName("");
    onClose();
  }

  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={handleCloseAll}>
      <Pressable style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={handleCloseAll}>
        <Pressable
          style={[styles.sheet, { backgroundColor: theme.colors.surface, borderTopLeftRadius: theme.radii.lg, borderTopRightRadius: theme.radii.lg }]}
          onPress={(e) => e.stopPropagation()}
        >
          <View style={styles.handle}>
            <View style={[styles.handleBar, { backgroundColor: theme.colors.textMuted }]} />
          </View>
          <Text style={[styles.title, { color: theme.colors.text }]}>{t("common.addToPlaylist")}</Text>

          {isLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginTop: 24 }} />
          ) : isError ? (
            <LoadFailed inline onRetry={() => void refetch()} />
          ) : manualPlaylists.length === 0 && !creating ? (
            <Text style={[styles.empty, { color: theme.colors.textMuted }]}>
              {t("common.noManualPlaylists")}
            </Text>
          ) : (
            <FlatList
              data={manualPlaylists}
              keyExtractor={(item) => item.id}
              style={{ maxHeight: 320 }}
              renderItem={({ item }) => {
                const added = addedIds.has(item.id);
                return (
                  <TouchableOpacity
                    style={[styles.row, { borderBottomColor: theme.colors.border }]}
                    activeOpacity={0.7}
                    onPress={() => !added && handleAdd(item.id)}
                    disabled={added || addTo.isPending}
                  >
                    <Ionicons name="list" size={ICON_SIZE.lg} color={theme.colors.accent} />
                    <Text style={[styles.playlistName, { color: theme.colors.text }]} numberOfLines={1}>
                      {item.name}
                    </Text>
                    {added ? (
                      <Ionicons name="checkmark-circle" size={ICON_SIZE.lg} color={theme.colors.accent} />
                    ) : (
                      <Ionicons name="add-circle-outline" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
                    )}
                  </TouchableOpacity>
                );
              }}
            />
          )}

          {creating ? (
            <View style={styles.createRow}>
              <TextInput
                value={newName}
                onChangeText={setNewName}
                placeholder={t("media.music.createPlaylistPrompt")}
                placeholderTextColor={theme.colors.textMuted}
                autoFocus
                accessibilityLabel={t("media.music.createPlaylistPrompt")}
                style={[
                  styles.input,
                  { color: theme.colors.text, borderColor: theme.colors.border, backgroundColor: theme.colors.surfaceRaised },
                ]}
                onSubmitEditing={handleCreateAndAdd}
                returnKeyType="done"
                editable={!createPl.isPending && !addTo.isPending}
              />
              <TouchableOpacity
                onPress={handleCreateAndAdd}
                disabled={!newName.trim() || createPl.isPending || addTo.isPending}
                style={[styles.createBtn, { backgroundColor: theme.colors.accent, opacity: !newName.trim() ? 0.5 : 1 }]}
                accessibilityRole="button"
                accessibilityLabel={t("media.playlist.createPlaylist")}
                accessibilityState={{ disabled: !newName.trim() || createPl.isPending || addTo.isPending }}
              >
                {createPl.isPending ? (
                  <ActivityIndicator color={theme.colors.onAccent} size="small" />
                ) : (
                  <Ionicons name="checkmark" size={ICON_SIZE.lg} color={theme.colors.onAccent} />
                )}
              </TouchableOpacity>
            </View>
          ) : (
            <TouchableOpacity
              onPress={() => setCreating(true)}
              style={[styles.newPlaylistRow, { borderTopColor: theme.colors.border }]}
              activeOpacity={0.7}
            >
              <Ionicons name="add-circle" size={ICON_SIZE.lg} color={theme.colors.accent} />
              <Text style={[styles.newPlaylistText, { color: theme.colors.accent }]}>
                {t("media.music.newPlaylist")}
              </Text>
            </TouchableOpacity>
          )}

          <TouchableOpacity
            style={[styles.closeBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
            onPress={handleCloseAll}
          >
            <Text style={[styles.closeBtnText, { color: theme.colors.text }]}>{t("common.close", "Close")}</Text>
          </TouchableOpacity>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "flex-end" },
  sheet: { paddingBottom: 34, paddingHorizontal: 16 },
  handle: { alignItems: "center", paddingVertical: 10 },
  handleBar: { width: 40, height: 4, borderRadius: 2, opacity: 0.4 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700", marginBottom: 12 },
  empty: { fontSize: FONT_SIZE.sm, textAlign: "center", marginTop: 24 },
  row: { flexDirection: "row", alignItems: "center", paddingVertical: 14, gap: 12, borderBottomWidth: StyleSheet.hairlineWidth },
  playlistName: { flex: 1, fontSize: FONT_SIZE.sm, fontWeight: "500" },
  newPlaylistRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 14, borderTopWidth: StyleSheet.hairlineWidth, marginTop: 4 },
  newPlaylistText: { fontSize: FONT_SIZE.sm, fontWeight: "600" },
  createRow: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 12 },
  input: { flex: 1, borderWidth: StyleSheet.hairlineWidth, borderRadius: 8, paddingHorizontal: 12, paddingVertical: 10, fontSize: FONT_SIZE.sm },
  createBtn: { width: 44, height: 44, borderRadius: 8, alignItems: "center", justifyContent: "center" },
  closeBtn: { marginTop: 16, paddingVertical: 12, alignItems: "center" },
  closeBtnText: { fontSize: FONT_SIZE.sm, fontWeight: "600" },
});
