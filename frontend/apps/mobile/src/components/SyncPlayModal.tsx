import { useState } from "react";
import { View, Text, Pressable, TextInput, Modal, StyleSheet, ScrollView } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { usePlaylistList } from "../api/playlists";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface Props {
  visible: boolean;
  onClose: () => void;
  onCreateRoom: () => void;
  onJoinRoom: (code: string) => void;
  onCreateRoomFromPlaylist?: (playlistId: string, name?: string) => void;
  creating?: boolean;
  joining?: boolean;
  showPlaylistOption?: boolean;
}

export function SyncPlayModal({ visible, onClose, onCreateRoom, onJoinRoom, onCreateRoomFromPlaylist, creating, joining, showPlaylistOption }: Props) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [code, setCode] = useState("");
  const [mode, setMode] = useState<"single" | "playlist">("single");
  const { data: playlists } = usePlaylistList();

  function handleJoin() {
    const trimmed = code.trim();
    if (trimmed.length >= 5) onJoinRoom(trimmed);
  }

  // A hidden RN Modal keeps children mounted: this one re-rendered on every
  // player currentTime tick. Render nothing until opened. (The playlists
  // query above still runs while MOUNTED - the player mounts this sheet only
  // when open, which is what stops it during playback.)
  if (!visible) return null;

  return (
    <Modal transparent visible={visible} animationType="fade" onRequestClose={onClose}>
      <Pressable style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={onClose} />
      <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
        <Text style={[styles.title, { color: theme.colors.text }]}>
          {t("media.syncplay.title", "Watch Together")}
        </Text>

        {showPlaylistOption && onCreateRoomFromPlaylist && (playlists ?? []).length > 0 && (
          <View style={[styles.tabs, { borderColor: theme.colors.border }]}>
            <Pressable
              onPress={() => setMode("single")}
              style={[styles.tab, mode === "single" && { backgroundColor: theme.colors.surfaceRaised }]}
            >
              <Text style={{ color: mode === "single" ? theme.colors.accent : theme.colors.textMuted, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>
                {t("media.syncplay.createRoom", "Create room")}
              </Text>
            </Pressable>
            <Pressable
              onPress={() => setMode("playlist")}
              style={[styles.tab, mode === "playlist" && { backgroundColor: theme.colors.surfaceRaised }]}
            >
              <Text style={{ color: mode === "playlist" ? theme.colors.accent : theme.colors.textMuted, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>
                {t("media.syncplay.orPlaylist", "Or pick a playlist")}
              </Text>
            </Pressable>
          </View>
        )}

        {mode === "single" || !showPlaylistOption || !onCreateRoomFromPlaylist ? (
          <Pressable
            onPress={onCreateRoom}
            disabled={creating}
            style={[styles.btn, { backgroundColor: theme.colors.accent, opacity: creating ? 0.5 : 1 }]}
          >
            <Ionicons name="add-circle-outline" size={ICON_SIZE.lg} color={theme.colors.onAccent} />
            <Text style={[styles.btnText, { color: theme.colors.onAccent }]}>{t("media.syncplay.createRoom", "Create room")}</Text>
          </Pressable>
        ) : (
          <ScrollView style={styles.playlistList}>
            {(playlists ?? []).map((pl) => (
              <Pressable
                key={pl.id}
                onPress={() => onCreateRoomFromPlaylist(pl.id, pl.name)}
                disabled={creating}
                style={[styles.playlistRow, { borderColor: theme.colors.border, opacity: creating ? 0.5 : 1 }]}
              >
                <Ionicons name="list-outline" size={ICON_SIZE.md} color={theme.colors.text} />
                <Text style={[styles.playlistName, { color: theme.colors.text }]} numberOfLines={1}>{pl.name}</Text>
                <Ionicons name="chevron-forward" size={ICON_SIZE.md} color={theme.colors.textMuted} />
              </Pressable>
            ))}
          </ScrollView>
        )}

        <View style={[styles.divider, { backgroundColor: theme.colors.border }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs, paddingHorizontal: 8 }}>
            {t("common.or", "or")}
          </Text>
        </View>

        <Text style={[styles.label, { color: theme.colors.textMuted }]}>
          {t("media.syncplay.joinByCode", "Join with code")}
        </Text>
        <View style={{ flexDirection: "row", gap: 8 }}>
          <TextInput
            value={code}
            onChangeText={setCode}
            placeholder="12345"
            placeholderTextColor={theme.colors.textMuted}
            keyboardType="number-pad"
            maxLength={5}
            accessibilityLabel={t("media.syncplay.joinByCode", "Join with code")}
            style={[styles.input, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderColor: theme.colors.border, flex: 1 }]}
          />
          <Pressable
            onPress={handleJoin}
            disabled={joining || code.trim().length < 5}
            style={[styles.btn, { backgroundColor: theme.colors.accent, opacity: joining || code.trim().length < 5 ? 0.5 : 1, flex: 0, paddingHorizontal: 16 }]}
          >
            <Text style={[styles.btnText, { color: theme.colors.onAccent }]}>{t("media.syncplay.join", "Join")}</Text>
          </Pressable>
        </View>

        <Pressable onPress={onClose} style={{ alignSelf: "center", marginTop: 12 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{t("common.cancel")}</Text>
        </Pressable>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { ...StyleSheet.absoluteFillObject },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700", marginBottom: 16 },
  tabs: { flexDirection: "row", borderWidth: 1, borderRadius: 6, marginBottom: 12, overflow: "hidden" },
  tab: { flex: 1, paddingVertical: 8, alignItems: "center" },
  btn: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8, paddingVertical: 12, borderRadius: 8 },
  btnText: { fontWeight: "600", fontSize: FONT_SIZE.sm },
  playlistList: { maxHeight: 180 },
  playlistRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingHorizontal: 12, paddingVertical: 10, borderBottomWidth: 1 },
  playlistName: { flex: 1, fontSize: FONT_SIZE.sm },
  divider: { height: 1, marginVertical: 16, position: "relative", alignItems: "center", justifyContent: "center" },
  label: { fontSize: FONT_SIZE.xs, fontWeight: "600", textTransform: "uppercase", marginBottom: 8 },
  input: { paddingHorizontal: 14, paddingVertical: 10, borderRadius: 8, borderWidth: 1, fontSize: FONT_SIZE.lg, fontWeight: "700", letterSpacing: 4, textAlign: "center" },
});
