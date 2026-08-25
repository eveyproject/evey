import { useState, useCallback } from "react";
import {
  Text,
  Modal,
  FlatList,
  Pressable,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { usePlaylistList, useAddToPlaylist } from "../api/playlists";
import { FocusableCard } from "./FocusableCard";
import { FONT_SIZE } from "@evey/theme";

interface Props {
  visible: boolean;
  mediaItemId: string;
  onClose: () => void;
}

export function AddToPlaylistSheet({ visible, mediaItemId, onClose }: Props) {
  const { t } = useTranslation();
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
          onError: () => toast.error(t("media.playlist.addError")),
        },
      );
    },
    [addTo, mediaItemId, t],
  );

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={onClose} focusable={false}>
        <Pressable
          style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: 16 }]}
          onPress={(e) => e.stopPropagation()}
          focusable={false}
        >
          <Text style={[styles.title, { color: theme.colors.text }]}>{t("media.detail.addToPlaylist")}</Text>

          {isLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginTop: 24 }} />
          ) : manualPlaylists.length === 0 ? (
            <Text style={[styles.empty, { color: theme.colors.textMuted }]}>
              {t("common.noManualPlaylists", "No manual playlists available")}
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
                    disabled={added || addTo.isPending}
                    hasTVPreferredFocus={index === 0}
                    accessibilityLabel={added ? `${item.name}, ${t("common.added", "Added")}` : item.name}
                    style={[styles.row, { borderBottomColor: theme.colors.border }]}
                  >
                    <Text style={[styles.icon, { color: theme.colors.accent }]}>
                      {added ? "\u2713" : "\u266A"}
                    </Text>
                    <Text style={[styles.playlistName, { color: theme.colors.text }]} numberOfLines={1}>
                      {item.name}
                    </Text>
                    <Text style={{ color: added ? theme.colors.accent : theme.colors.textMuted, fontSize: FONT_SIZE.xl }}>
                      {added ? t("common.added", "Added") : "+"}
                    </Text>
                  </FocusableCard>
                );
              }}
            />
          )}

          <FocusableCard
            onPress={onClose}
            // While loading, or with no manual playlist to show, this is the
            // only focusable in the sheet: without the claim Android has no
            // focused view and stops delivering keys - the viewer cannot even
            // close it.
            hasTVPreferredFocus={isLoading || manualPlaylists.length === 0}
            accessibilityLabel={t("media.detail.close", "Close")}
            style={[styles.closeBtn, { backgroundColor: theme.colors.surfaceRaised }]}
          >
            <Text style={[styles.closeBtnText, { color: theme.colors.text }]}>{t("media.detail.close", "Close")}</Text>
          </FocusableCard>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "center", alignItems: "center" },
  sheet: { width: 500, padding: 32 },
  title: { fontSize: FONT_SIZE["3xl"], fontWeight: "700", marginBottom: 16 },
  empty: { fontSize: FONT_SIZE.xl, textAlign: "center", marginTop: 24 },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 16,
    paddingHorizontal: 12,
    gap: 16,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  icon: { fontSize: FONT_SIZE.xl, width: 32, textAlign: "center" },
  playlistName: { flex: 1, fontSize: FONT_SIZE.xl, fontWeight: "500" },
  closeBtn: { marginTop: 20, paddingVertical: 16, alignItems: "center", borderRadius: 8 },
  closeBtnText: { fontSize: FONT_SIZE.xl, fontWeight: "600" },
});
