import { useState } from "react";
import {
  View,
  Text,
  Modal,
  FlatList,
  TouchableOpacity,
  Pressable,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTheme } from "../providers/ThemeProvider";
import { usePlaylistList, useAddToPlaylist } from "../api/playlists";

interface Props {
  visible: boolean;
  mediaItemId: string;
  onClose: () => void;
}

export function AddToPlaylistSheet({ visible, mediaItemId, onClose }: Props) {
  const theme = useTheme();
  const { data: playlists, isLoading } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const [addedIds, setAddedIds] = useState<Set<string>>(new Set());

  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");

  function handleAdd(playlistId: string) {
    addTo.mutate(
      { playlistId, mediaItemId },
      {
        onSuccess: () => setAddedIds((prev) => new Set(prev).add(playlistId)),
      },
    );
  }

  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable
          style={[styles.sheet, { backgroundColor: theme.colors.surface, borderTopLeftRadius: theme.radii.lg, borderTopRightRadius: theme.radii.lg }]}
          onPress={(e) => e.stopPropagation()}
        >
          <View style={styles.handle}>
            <View style={[styles.handleBar, { backgroundColor: theme.colors.textMuted }]} />
          </View>
          <Text style={[styles.title, { color: theme.colors.text }]}>Add to Playlist</Text>

          {isLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginTop: 24 }} />
          ) : manualPlaylists.length === 0 ? (
            <Text style={[styles.empty, { color: theme.colors.textMuted }]}>
              No manual playlists available
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
                    <Ionicons name="list" size={18} color={theme.colors.accent} />
                    <Text style={[styles.playlistName, { color: theme.colors.text }]} numberOfLines={1}>
                      {item.name}
                    </Text>
                    {added ? (
                      <Ionicons name="checkmark-circle" size={20} color={theme.colors.accent} />
                    ) : (
                      <Ionicons name="add-circle-outline" size={20} color={theme.colors.textMuted} />
                    )}
                  </TouchableOpacity>
                );
              }}
            />
          )}

          <TouchableOpacity
            style={[styles.closeBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
            onPress={onClose}
          >
            <Text style={[styles.closeBtnText, { color: theme.colors.text }]}>Close</Text>
          </TouchableOpacity>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "flex-end", backgroundColor: "rgba(0,0,0,0.5)" },
  sheet: { paddingBottom: 34, paddingHorizontal: 16 },
  handle: { alignItems: "center", paddingVertical: 10 },
  handleBar: { width: 40, height: 4, borderRadius: 2, opacity: 0.4 },
  title: { fontSize: 18, fontWeight: "700", marginBottom: 12 },
  empty: { fontSize: 14, textAlign: "center", marginTop: 24 },
  row: { flexDirection: "row", alignItems: "center", paddingVertical: 14, gap: 12, borderBottomWidth: StyleSheet.hairlineWidth },
  playlistName: { flex: 1, fontSize: 15, fontWeight: "500" },
  closeBtn: { marginTop: 16, paddingVertical: 12, alignItems: "center" },
  closeBtnText: { fontSize: 15, fontWeight: "600" },
});
