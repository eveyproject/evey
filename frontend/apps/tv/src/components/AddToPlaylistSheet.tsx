import { useState, useCallback } from "react";
import {
  View,
  Text,
  Modal,
  FlatList,
  Pressable,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useTheme } from "../providers/ThemeProvider";
import { usePlaylistList, useAddToPlaylist } from "../api/playlists";
import { FocusableCard } from "./FocusableCard";

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

  const handleAdd = useCallback(
    (playlistId: string) => {
      addTo.mutate(
        { playlistId, mediaItemId },
        {
          onSuccess: () => setAddedIds((prev) => new Set(prev).add(playlistId)),
        },
      );
    },
    [addTo, mediaItemId],
  );

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable
          style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: 16 }]}
          onPress={(e) => e.stopPropagation()}
        >
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
              style={{ maxHeight: 400 }}
              renderItem={({ item, index }) => {
                const added = addedIds.has(item.id);
                return (
                  <FocusableCard
                    onPress={() => !added && handleAdd(item.id)}
                    hasTVPreferredFocus={index === 0}
                    style={[styles.row, { borderBottomColor: theme.colors.border }]}
                  >
                    <Text style={[styles.icon, { color: theme.colors.accent }]}>
                      {added ? "\u2713" : "\u266A"}
                    </Text>
                    <Text style={[styles.playlistName, { color: theme.colors.text }]} numberOfLines={1}>
                      {item.name}
                    </Text>
                    <Text style={{ color: added ? theme.colors.accent : theme.colors.textMuted, fontSize: 20 }}>
                      {added ? "Added" : "+"}
                    </Text>
                  </FocusableCard>
                );
              }}
            />
          )}

          <FocusableCard
            onPress={onClose}
            style={[styles.closeBtn, { backgroundColor: theme.colors.surfaceRaised }]}
          >
            <Text style={[styles.closeBtnText, { color: theme.colors.text }]}>Close</Text>
          </FocusableCard>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: "rgba(0,0,0,0.7)" },
  sheet: { width: 500, padding: 32 },
  title: { fontSize: 28, fontWeight: "700", marginBottom: 16 },
  empty: { fontSize: 20, textAlign: "center", marginTop: 24 },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 16,
    paddingHorizontal: 12,
    gap: 16,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  icon: { fontSize: 22, width: 32, textAlign: "center" },
  playlistName: { flex: 1, fontSize: 20, fontWeight: "500" },
  closeBtn: { marginTop: 20, paddingVertical: 16, alignItems: "center", borderRadius: 8 },
  closeBtnText: { fontSize: 20, fontWeight: "600" },
});
