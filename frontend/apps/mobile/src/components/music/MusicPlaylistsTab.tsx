import { useCallback, useEffect, useMemo, useState } from "react";
import { formatTime } from "@evey/api";
import { View, Text, FlatList, TouchableOpacity, Pressable, Alert, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { useAuthHeaders } from "../../hooks/useAuthenticatedImage";
import { useServerUrl } from "../../hooks/useServerUrl";
import { toast } from "@evey/store";
import { usePlaylistList, usePlaylistDetail, useDeletePlaylist, useGeneratePlaylist, useRemoveFromPlaylist } from "../../api/playlists";
import { useMusicQueueStore, type QueueTrack } from "../../stores/musicQueue";
import { TrackArt } from "./TrackArt";
import type { Track } from "./musicTypes";
import { shuffleArray } from "../../lib/shuffleArray";

function toQueueTrack(tr: Track): QueueTrack {
  return { id: tr.id, title: tr.title, artist: tr.artist, album: tr.album, duration: tr.duration };
}

interface Props {
  // The already-loaded library track list (from the tracks tab's query) -
  // playlist items only carry an id/title/type/year, so the full track shape
  // (artist, album, duration, cover) is joined from this list, same as the
  // web app's PlaylistsTab joining against its own `allTracks`.
  libraryTracks: Track[];
  // Opens straight into this playlist when set - used by the music screen to
  // restore the exact playlist view a queue's "back to source" link points
  // to. Not a plain useState initializer: this tab stays mounted across
  // repeat navigations to a *different* target playlist, so it must react to
  // prop changes rather than only seed the initial render.
  initialSelectedId?: string | null;
  // Enables the server-side "generate playlist" button (web parity); the
  // endpoint needs a music library to sample from.
  libraryId?: string;
}

export function MusicPlaylistsTab({ libraryTracks, initialSelectedId = null, libraryId }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const [selectedId, setSelectedId] = useState<string | null>(initialSelectedId);

  useEffect(() => {
    if (initialSelectedId) setSelectedId(initialSelectedId);
  }, [initialSelectedId]);

  const { data: playlists, isLoading: listLoading } = usePlaylistList();
  const { data: detail, isLoading: detailLoading } = usePlaylistDetail(selectedId ?? "");
  const removeItem = useRemoveFromPlaylist();
  const deletePlaylist = useDeletePlaylist();
  const generate = useGeneratePlaylist();

  const handleGenerate = useCallback(() => {
    if (!libraryId || generate.isPending) return;
    generate.mutate(
      { libraryId, name: t("media.playlist.generatedName", "My mix") },
      {
        onSuccess: (res) => {
          toast.success(t("media.playlist.generated", "Playlist generated"));
          if (res?.id) setSelectedId(res.id);
        },
        onError: () => toast.error(t("media.playlist.generateFailed", "Could not generate playlist")),
      },
    );
  }, [libraryId, generate, t]);

  // Smart playlists are rule-based (no rules editor ported to mobile yet) -
  // only manual playlists can be browsed/played here, mirrors the web app's
  // Playlists tab filter.
  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");

  const tracksByID = useMemo(() => {
    const m = new Map<string, Track>();
    libraryTracks.forEach((tr) => m.set(tr.id, tr));
    return m;
  }, [libraryTracks]);

  const playlistTracks: Track[] = useMemo(
    () =>
      (detail?.items ?? [])
        .map((it) => tracksByID.get(it.media_item_id))
        .filter((x): x is Track => !!x),
    [detail, tracksByID],
  );

  const handlePlay = useCallback(
    (index: number) => {
      if (!detail) return;
      setQueue(playlistTracks.map(toQueueTrack), index, { type: "playlist", id: detail.id, label: detail.name });
    },
    [detail, playlistTracks, setQueue],
  );

  const handleShuffle = useCallback(() => {
    if (!detail || playlistTracks.length === 0) return;
    const shuffled = shuffleArray(playlistTracks).map(toQueueTrack);
    setQueue(shuffled, 0, { type: "playlist", id: detail.id, label: detail.name });
  }, [detail, playlistTracks, setQueue]);

  const handleRemove = useCallback(
    (trackID: string) => {
      if (!selectedId) return;
      removeItem.mutate({ playlistId: selectedId, mediaItemId: trackID });
    },
    [selectedId, removeItem],
  );

  const handleDelete = useCallback(
    (playlist: { id: string; name: string }) => {
      Alert.alert(t("media.playlist.deletePlaylist"), t("media.playlist.deleteConfirm", { name: playlist.name }), [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t("common.delete"),
          style: "destructive",
          onPress: () => {
            deletePlaylist.mutate(playlist.id);
            setSelectedId((cur) => (cur === playlist.id ? null : cur));
          },
        },
      ]);
    },
    [t, deletePlaylist],
  );

  if (selectedId) {
    return (
      <View style={{ flex: 1 }}>
        <View style={[styles.detailHeader, { paddingHorizontal: theme.spacing.md }]}>
          <Pressable onPress={() => setSelectedId(null)} hitSlop={8} accessibilityLabel={t("common.back")}>
            <Ionicons name="arrow-back" size={22} color={theme.colors.text} />
          </Pressable>
          <Text style={[styles.detailTitle, { color: theme.colors.text }]} numberOfLines={1}>
            {detail?.name ?? ""}
          </Text>
          {playlistTracks.length > 0 && (
            <TouchableOpacity
              onPress={handleShuffle}
              testID="playlist-shuffle-play"
              style={[styles.shuffleBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.sm }]}
            >
              <Ionicons name="shuffle" size={16} color={theme.colors.onAccent} />
              <Text style={{ color: theme.colors.onAccent, fontSize: 13, fontWeight: "600" }}>{t("media.music.shuffle")}</Text>
            </TouchableOpacity>
          )}
        </View>
        {detailLoading ? (
          <Text style={[styles.centerText, { color: theme.colors.textMuted }]}>{t("media.music.loading")}</Text>
        ) : (
          <FlatList
            data={playlistTracks}
            keyExtractor={(item, i) => `${item.id}-${i}`}
            contentContainerStyle={{ paddingBottom: 32 }}
            ListEmptyComponent={
              <Text style={[styles.centerText, { color: theme.colors.textMuted }]}>{t("media.music.emptyPlaylist")}</Text>
            }
            renderItem={({ item, index }) => (
              <TouchableOpacity
                activeOpacity={0.7}
                onPress={() => handlePlay(index)}
                style={[styles.trackRow, { borderBottomColor: theme.colors.surface }]}
              >
                <TrackArt trackID={item.id} size={36} authHeaders={authHeaders} serverUrl={serverUrl} theme={theme} />
                <View style={styles.trackInfo}>
                  <Text style={[styles.trackTitle, { color: theme.colors.text }]} numberOfLines={1}>
                    {item.title || "-"}
                  </Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
                    {item.artist || "-"}
                  </Text>
                </View>
                {item.duration > 0 && (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{formatTime(item.duration)}</Text>
                )}
                <Pressable
                  onPress={(e) => {
                    e.stopPropagation();
                    handleRemove(item.id);
                  }}
                  hitSlop={8}
                  style={styles.iconBtn}
                  accessibilityLabel={t("common.delete")}
                >
                  <Ionicons name="close-circle-outline" size={20} color={theme.colors.textMuted} />
                </Pressable>
              </TouchableOpacity>
            )}
          />
        )}
      </View>
    );
  }

  if (listLoading) {
    return <Text style={[styles.centerText, { color: theme.colors.textMuted }]}>{t("media.music.loading")}</Text>;
  }

  return (
    <FlatList
      data={manualPlaylists}
      keyExtractor={(item) => item.id}
      contentContainerStyle={{ padding: 16, gap: 8 }}
      ListHeaderComponent={
        libraryId ? (
          <TouchableOpacity
            onPress={handleGenerate}
            disabled={generate.isPending}
            testID="generate-playlist-btn"
            style={[
              styles.generateBtn,
              { backgroundColor: theme.colors.accent, borderRadius: theme.radii.sm, opacity: generate.isPending ? 0.5 : 1 },
            ]}
          >
            <Ionicons name="sparkles" size={16} color={theme.colors.onAccent} />
            <Text style={{ color: theme.colors.onAccent, fontSize: 13, fontWeight: "600" }}>
              {generate.isPending ? t("media.playlist.generating", "Generating...") : t("media.playlist.generate", "Generate playlist")}
            </Text>
          </TouchableOpacity>
        ) : null
      }
      ListEmptyComponent={
        <Text style={[styles.centerText, { color: theme.colors.textMuted }]}>{t("media.music.noPlaylists")}</Text>
      }
      renderItem={({ item }) => (
        <View style={[styles.playlistRow, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
          <TouchableOpacity activeOpacity={0.7} onPress={() => setSelectedId(item.id)} style={{ flex: 1 }}>
            <Text style={[styles.playlistName, { color: theme.colors.text }]} numberOfLines={1}>
              {item.name || "-"}
            </Text>
            {!!item.description && (
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
                {item.description}
              </Text>
            )}
          </TouchableOpacity>
          <Pressable
            onPress={() => handleDelete({ id: item.id, name: item.name })}
            hitSlop={8}
            style={styles.iconBtn}
            accessibilityLabel={t("common.delete")}
          >
            <Ionicons name="trash-outline" size={18} color={theme.colors.textMuted} />
          </Pressable>
        </View>
      )}
    />
  );
}

const styles = StyleSheet.create({
  centerText: { textAlign: "center", marginTop: 24, fontSize: 15 },
  detailHeader: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 10 },
  detailTitle: { flex: 1, fontSize: 17, fontWeight: "700" },
  shuffleBtn: { flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: 12, paddingVertical: 7 },
  generateBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 6,
    paddingHorizontal: 12,
    paddingVertical: 9,
    marginBottom: 8,
  },
  trackRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 10,
    gap: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  trackInfo: { flex: 1 },
  trackTitle: { fontSize: 14, fontWeight: "500" },
  iconBtn: { padding: 6 },
  playlistRow: { flexDirection: "row", alignItems: "center", paddingHorizontal: 14, paddingVertical: 12 },
  playlistName: { fontSize: 15, fontWeight: "600" },
});
