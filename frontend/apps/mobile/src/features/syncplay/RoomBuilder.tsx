import { useCallback, useMemo, useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useMusicServiceGetAlbums, useMusicServiceListTracks } from "@evey/api";
import {
  addToRoomQueue,
  removeFromRoomQueue,
  moveInRoomQueue,
  albumRoomQueueItems,
  playlistRoomQueueItems,
  usePlaylistList,
  type RoomQueueItem,
} from "@evey/client";
import { toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { useDebounce } from "../../hooks/useDebounce";

type Tab = "albums" | "tracks" | "playlists";

// The phone's "listen together" room builder, same model as the web one: the
// host mixes whole albums, single tracks and playlists into one queue, then
// creates the room. Lists are capped to what fits a phone screen; the queue is
// the shared @evey/client model (no duplicates, 500 max).
export function RoomBuilder({
  libraryId,
  creating,
  onCreate,
}: {
  libraryId: string | undefined;
  creating: boolean;
  onCreate: (items: RoomQueueItem[]) => void;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("albums");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebounce(search, 300);
  const [queue, setQueue] = useState<RoomQueueItem[]>([]);
  const [busy, setBusy] = useState(false);

  const { data: albumsResp } = useMusicServiceGetAlbums({ libraryId }, { query: { enabled: !!libraryId, staleTime: 60_000 } });
  const albums = albumsResp?.albums ?? [];
  const { data: tracksResp } = useMusicServiceListTracks(
    { libraryId, limit: 100, search: debouncedSearch || undefined },
    { query: { enabled: !!libraryId && tab === "tracks", staleTime: 30_000 } },
  );
  const tracks = useMemo(
    () => (tracksResp?.tracks ?? []).map((tr): RoomQueueItem => ({ id: tr.id ?? "", title: tr.title ?? "", artist: tr.artist ?? "" })),
    [tracksResp],
  );
  const { data: playlists } = usePlaylistList();
  const queued = useMemo(() => new Set(queue.map((q) => q.id)), [queue]);

  const add = useCallback((items: RoomQueueItem[]) => setQueue((prev) => addToRoomQueue(prev, items)), []);

  const addAlbum = useCallback(async (albumId: string) => {
    if (!libraryId || !albumId) return;
    setBusy(true);
    try {
      add(await albumRoomQueueItems(libraryId, albumId));
    } catch {
      toast.error(t("media.syncplay.addAlbumFailed"));
    } finally {
      setBusy(false);
    }
  }, [libraryId, add, t]);

  const addPlaylist = useCallback(async (playlistId: string) => {
    setBusy(true);
    try {
      add(await playlistRoomQueueItems(playlistId));
    } catch {
      toast.error(t("media.syncplay.addPlaylistFailed"));
    } finally {
      setBusy(false);
    }
  }, [add, t]);

  const rowStyle = [styles.row, { borderBottomColor: theme.colors.border }];
  const addButton = (label: string, onPress: () => void, disabled = false, testID?: string) => (
    <Pressable onPress={onPress} disabled={disabled || busy} hitSlop={8} accessibilityRole="button" accessibilityLabel={label} testID={testID}>
      <Ionicons name={disabled ? "checkmark" : "add-circle-outline"} size={22} color={disabled ? theme.colors.textMuted : theme.colors.accent} />
    </Pressable>
  );

  return (
    <View style={{ gap: 12 }}>
      <View style={{ flexDirection: "row", gap: 8 }} accessibilityRole="tablist">
        {(["albums", "tracks", "playlists"] as Tab[]).map((key) => (
          <Pressable
            key={key}
            onPress={() => setTab(key)}
            accessibilityRole="tab"
            accessibilityState={{ selected: tab === key }}
            style={[styles.tab, { backgroundColor: tab === key ? theme.colors.accent : theme.colors.surface, borderRadius: theme.radii.sm }]}
          >
            <Text style={{ color: tab === key ? theme.colors.onAccent : theme.colors.textMuted, fontWeight: "600", fontSize: 13 }}>
              {t(`media.syncplay.tab${key.charAt(0).toUpperCase()}${key.slice(1)}`)}
            </Text>
          </Pressable>
        ))}
      </View>

      {tab === "tracks" && (
        <TextInput
          value={search}
          onChangeText={setSearch}
          placeholder={t("media.search.placeholder")}
          placeholderTextColor={theme.colors.textMuted}
          autoCapitalize="none"
          autoCorrect={false}
          style={[styles.search, { backgroundColor: theme.colors.surface, color: theme.colors.text, borderRadius: theme.radii.sm }]}
        />
      )}

      <View>
        {tab === "albums" && (albums.length === 0
          ? <Text style={{ color: theme.colors.textMuted }}>{t("media.syncplay.noAlbums")}</Text>
          : albums.slice(0, 100).map((a) => (
            <View key={a.id} style={rowStyle}>
              <View style={{ flex: 1 }}>
                <Text style={{ color: theme.colors.text }} numberOfLines={1}>{a.title}</Text>
                <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>{a.artist}</Text>
              </View>
              {addButton(t("media.syncplay.addAlbum"), () => { void addAlbum(a.id ?? ""); }, false, `room-add-album-${a.id}`)}
            </View>
          )))}
        {tab === "tracks" && (tracks.length === 0
          ? <Text style={{ color: theme.colors.textMuted }}>{t("media.syncplay.noTracks")}</Text>
          : tracks.map((tr) => (
            <View key={tr.id} style={rowStyle}>
              <View style={{ flex: 1 }}>
                <Text style={{ color: theme.colors.text }} numberOfLines={1}>{tr.title}</Text>
                <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>{tr.artist}</Text>
              </View>
              {addButton(t("media.syncplay.addToQueue"), () => add([tr]), queued.has(tr.id), `room-add-track-${tr.id}`)}
            </View>
          )))}
        {tab === "playlists" && ((playlists ?? []).length === 0
          ? <Text style={{ color: theme.colors.textMuted }}>{t("media.syncplay.noPlaylists")}</Text>
          : (playlists ?? []).map((p) => (
            <View key={p.id} style={rowStyle}>
              <Text style={{ color: theme.colors.text, flex: 1 }} numberOfLines={1}>{p.name}</Text>
              {addButton(t("media.syncplay.addToQueue"), () => { void addPlaylist(p.id); }, false, `room-add-playlist-${p.id}`)}
            </View>
          )))}
      </View>

      <View style={{ gap: 6 }}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
          <Ionicons name="list" size={16} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.text, fontWeight: "600", flex: 1 }}>{t("media.syncplay.queue")} ({queue.length})</Text>
          {busy && <ActivityIndicator size="small" color={theme.colors.accent} />}
          {queue.length > 0 && (
            <Pressable onPress={() => setQueue([])} accessibilityRole="button" hitSlop={8}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("media.syncplay.clearQueue")}</Text>
            </Pressable>
          )}
        </View>
        {queue.length === 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("media.syncplay.queueEmpty")}</Text>
        ) : (
          queue.map((q, idx) => (
            <View key={q.id} style={rowStyle} testID={`room-queue-${q.id}`}>
              <Text style={{ color: theme.colors.textMuted, width: 24, fontSize: 12 }}>{idx + 1}</Text>
              <Text style={{ color: theme.colors.text, flex: 1 }} numberOfLines={1}>{q.title || q.id}</Text>
              <Pressable onPress={() => setQueue((prev) => moveInRoomQueue(prev, idx, -1))} disabled={idx === 0} hitSlop={6} accessibilityRole="button" accessibilityLabel={t("media.syncplay.moveUp")}>
                <Ionicons name="chevron-up" size={18} color={idx === 0 ? theme.colors.border : theme.colors.textMuted} />
              </Pressable>
              <Pressable onPress={() => setQueue((prev) => moveInRoomQueue(prev, idx, 1))} disabled={idx === queue.length - 1} hitSlop={6} accessibilityRole="button" accessibilityLabel={t("media.syncplay.moveDown")}>
                <Ionicons name="chevron-down" size={18} color={idx === queue.length - 1 ? theme.colors.border : theme.colors.textMuted} />
              </Pressable>
              <Pressable onPress={() => setQueue((prev) => removeFromRoomQueue(prev, q.id))} hitSlop={6} accessibilityRole="button" accessibilityLabel={t("media.syncplay.removeFromQueue")}>
                <Ionicons name="close" size={18} color={theme.colors.error} />
              </Pressable>
            </View>
          ))
        )}
      </View>

      <Pressable
        onPress={() => onCreate(queue)}
        disabled={queue.length === 0 || creating}
        accessibilityRole="button"
        accessibilityLabel={t("media.syncplay.createRoom")}
        testID="room-create"
        style={[styles.create, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: queue.length === 0 || creating ? 0.5 : 1 }]}
      >
        {creating ? <ActivityIndicator color={theme.colors.onAccent} /> : <Ionicons name="play" size={18} color={theme.colors.onAccent} />}
        <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("media.syncplay.createRoom")}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  tab: { paddingHorizontal: 14, paddingVertical: 8 },
  search: { paddingHorizontal: 12, paddingVertical: 10, fontSize: 14 },
  row: { flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth },
  create: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8, paddingVertical: 12 },
});
