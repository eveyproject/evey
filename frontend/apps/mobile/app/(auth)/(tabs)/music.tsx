import { useState, useCallback } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  Pressable,
  ScrollView,
  Modal,
  Dimensions,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { Skeleton } from "../../../src/components/Skeleton";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useMusicQueueStore, type QueueTrack } from "../../../src/stores/musicQueue";

interface LibraryItem {
  id: string;
  name: string;
  type: string;
}

interface Track {
  ID: string;
  Title: string;
  Artist: string;
  Album: string;
  Duration: number;
  TrackNumber: number;
}

interface MusicAlbum {
  ID: string;
  Title: string;
  Artist: string;
  Year: number;
  TrackCount: number;
}

interface MusicArtist {
  ID: string;
  Name: string;
  TrackCount: number;
  AlbumCount: number;
}

type Tab = "tracks" | "albums" | "artists";

const NUM_ALBUM_COLS = 2;
const screenWidth = Dimensions.get("window").width;
const ALBUM_ART_SIZE = (screenWidth - 16 * 3) / NUM_ALBUM_COLS;

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${String(s).padStart(2, "0")}`;
}

function useMusicLibraries() {
  return useQuery({
    queryKey: ["libraries", "music"],
    queryFn: () => eveyFetch<{ libraries: LibraryItem[] }>({ url: "/api/v1/libraries", method: "GET" }),
    select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC"),
    staleTime: 60_000,
  });
}

function useMusicTracks(libraryID: string | undefined) {
  return useQuery({
    queryKey: ["music", "tracks", libraryID],
    queryFn: () => eveyFetch<{ tracks: Track[] }>({ url: `/api/v1/music/tracks?library_id=${libraryID}&limit=200`, method: "GET" }),
    enabled: !!libraryID,
    select: (d) => d.tracks ?? [],
    staleTime: 60_000,
  });
}

function useMusicAlbums(libraryID: string | undefined) {
  return useQuery({
    queryKey: ["music", "albums", libraryID],
    queryFn: () => eveyFetch<{ albums: MusicAlbum[] }>({ url: `/api/v1/music/albums?library_id=${libraryID}`, method: "GET" }),
    enabled: !!libraryID,
    select: (d) => d.albums ?? [],
    staleTime: 60_000,
  });
}

function useMusicArtists(libraryID: string | undefined) {
  return useQuery({
    queryKey: ["music", "artists", libraryID],
    queryFn: () => eveyFetch<{ artists: MusicArtist[] }>({ url: `/api/v1/music/artists?library_id=${libraryID}`, method: "GET" }),
    enabled: !!libraryID,
    select: (d) => d.artists ?? [],
    staleTime: 60_000,
  });
}

export default function MusicScreen() {
  const theme = useTheme();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();

  const [tab, setTab] = useState<Tab>("tracks");
  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [showQueue, setShowQueue] = useState(false);

  const queue = useMusicQueueStore((s) => s.queue);
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const playTrack = useMusicQueueStore((s) => s.playTrack);
  const removeFromQueue = useMusicQueueStore((s) => s.removeFromQueue);

  const handlePlayTrack = useCallback((index: number) => {
    const allTracks = (tracks ?? []).map((t): QueueTrack => ({
      id: t.ID,
      title: t.Title,
      artist: t.Artist,
      album: t.Album,
      duration: t.Duration,
    }));
    setQueue(allTracks, index);
  }, [tracks, setQueue]);

  const { data: libraries, isLoading: libLoading } = useMusicLibraries();
  const activeLibraryID = selectedLibraryID ?? libraries?.[0]?.id;
  const { data: tracks, isLoading: tracksLoading } = useMusicTracks(activeLibraryID);
  const { data: albums, isLoading: albumsLoading } = useMusicAlbums(activeLibraryID);
  const { data: artists, isLoading: artistsLoading } = useMusicArtists(activeLibraryID);

  const albumArtUri = useCallback(
    (albumID: string) => (serverUrl ? `${serverUrl}/api/v1/music/albums/${albumID}/artwork` : undefined),
    [serverUrl],
  );

  const renderTrack = useCallback(
    ({ item, index }: { item: Track; index: number }) => {
      const isCurrentTrack = currentIndex === index && queue.length > 0;
      return (
      <TouchableOpacity
        activeOpacity={0.7}
        onPress={() => handlePlayTrack(index)}
        style={[styles.trackRow, { borderBottomColor: theme.colors.surface, backgroundColor: isCurrentTrack ? theme.colors.surface : "transparent" }]}
      >
        <View style={styles.trackIndex}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{item.TrackNumber || index + 1}</Text>
        </View>
        <View style={styles.trackInfo}>
          <Text style={[styles.trackTitle, { color: theme.colors.text }]} numberOfLines={1}>{item.Title || "—"}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
            {[item.Artist, item.Album].filter(Boolean).join(" · ") || "—"}
          </Text>
        </View>
        {item.Duration > 0 && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{formatDuration(item.Duration)}</Text>
        )}
      </TouchableOpacity>
    );},
    [theme, currentIndex, queue, handlePlayTrack],
  );

  const renderAlbum = useCallback(
    ({ item }: { item: MusicAlbum }) => {
      const uri = albumArtUri(item.ID);
      return (
        <TouchableOpacity
          activeOpacity={0.7}
          style={[styles.albumCard, { width: ALBUM_ART_SIZE, backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        >
          {uri ? (
            <Image
              source={{ uri, headers: authHeaders }}
              style={{ width: ALBUM_ART_SIZE, height: ALBUM_ART_SIZE, borderRadius: theme.radii.md }}
              contentFit="cover"
              transition={150}
            />
          ) : (
            <View style={[styles.artPlaceholder, { width: ALBUM_ART_SIZE, height: ALBUM_ART_SIZE, borderRadius: theme.radii.md, backgroundColor: theme.colors.surfaceRaised }]}>
              <Text style={{ fontSize: 36 }}>🎵</Text>
            </View>
          )}
          <View style={{ padding: 8 }}>
            <Text style={[styles.albumTitle, { color: theme.colors.text }]} numberOfLines={1}>{item.Title || "—"}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>{item.Artist || "—"}</Text>
            {item.Year > 0 && (
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.Year} · {item.TrackCount} track{item.TrackCount !== 1 ? "s" : ""}</Text>
            )}
          </View>
        </TouchableOpacity>
      );
    },
    [albumArtUri, authHeaders, theme],
  );

  if (libLoading) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl, paddingHorizontal: theme.spacing.md }]}>Music</Text>
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.lg }}>
          {Array.from({ length: 6 }).map((_, i) => (
            <View key={i} style={{ marginBottom: 12 }}>
              <Skeleton width="100%" height={44} borderRadius={8} />
            </View>
          ))}
        </View>
      </View>
    );
  }

  if (!libraries || libraries.length === 0) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ fontSize: 32, marginBottom: 12 }}>🎵</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>No music libraries</Text>
      </View>
    );
  }

  const renderArtist = useCallback(
    ({ item }: { item: MusicArtist }) => (
      <TouchableOpacity
        activeOpacity={0.7}
        style={[styles.artistCard, { width: ALBUM_ART_SIZE, backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      >
        <View style={[styles.artistAvatar, { width: ALBUM_ART_SIZE * 0.6, height: ALBUM_ART_SIZE * 0.6, backgroundColor: theme.colors.surfaceRaised }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 24, fontWeight: "700" }}>
            {item.Name?.[0]?.toUpperCase() ?? "?"}
          </Text>
        </View>
        <Text style={[styles.artistName, { color: theme.colors.text }]} numberOfLines={2}>{item.Name || "—"}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
          {item.AlbumCount} album{item.AlbumCount !== 1 ? "s" : ""} · {item.TrackCount} track{item.TrackCount !== 1 ? "s" : ""}
        </Text>
      </TouchableOpacity>
    ),
    [theme],
  );

  const isLoading = tab === "tracks" ? tracksLoading : tab === "albums" ? albumsLoading : artistsLoading;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl, paddingHorizontal: theme.spacing.md }]}>Music</Text>

      {/* Library picker */}
      {libraries.length > 1 && (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.sm }}>
          {libraries.map((lib) => (
            <TouchableOpacity
              key={lib.id}
              activeOpacity={0.7}
              onPress={() => setSelectedLibraryID(lib.id)}
              style={[
                styles.libChip,
                {
                  backgroundColor: activeLibraryID === lib.id ? theme.colors.accent : theme.colors.surface,
                  borderRadius: theme.radii.sm,
                  marginRight: theme.spacing.sm,
                },
              ]}
            >
              <Text style={{ color: activeLibraryID === lib.id ? "#fff" : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                {lib.name}
              </Text>
            </TouchableOpacity>
          ))}
        </ScrollView>
      )}

      {/* Tab switcher */}
      <View style={[styles.tabBar, { marginHorizontal: theme.spacing.md, marginTop: theme.spacing.md, backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}>
        {(["tracks", "albums", "artists"] as Tab[]).map((t) => (
          <TouchableOpacity
            key={t}
            activeOpacity={0.8}
            onPress={() => setTab(t)}
            style={[
              styles.tabButton,
              {
                backgroundColor: tab === t ? theme.colors.accent : "transparent",
                borderRadius: theme.radii.sm,
              },
            ]}
          >
            <Text style={{ color: tab === t ? "#fff" : theme.colors.textMuted, fontSize: 14, fontWeight: "600", textTransform: "capitalize" }}>
              {t}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      {/* Queue button */}
      {queue.length > 0 && (
        <Pressable
          onPress={() => setShowQueue(true)}
          style={{ flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: theme.spacing.md, paddingVertical: 6 }}
        >
          <Ionicons name="list" size={16} color={theme.colors.accent} />
          <Text style={{ color: theme.colors.accent, fontSize: 13 }}>Queue ({queue.length})</Text>
        </Pressable>
      )}

      {/* Content */}
      {isLoading ? (
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.lg }}>
          {tab === "tracks"
            ? Array.from({ length: 8 }).map((_, i) => (
                <View key={i} style={{ marginBottom: 10 }}>
                  <Skeleton width="100%" height={48} borderRadius={8} />
                </View>
              ))
            : (
              <View style={styles.albumsGrid}>
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} width={ALBUM_ART_SIZE} height={ALBUM_ART_SIZE + 60} borderRadius={theme.radii.md} />
                ))}
              </View>
            )}
        </View>
      ) : tab === "tracks" ? (
        <FlatList
          data={tracks ?? []}
          keyExtractor={(item) => item.ID}
          renderItem={renderTrack}
          contentContainerStyle={{ paddingBottom: 32 }}
          initialNumToRender={20}
          maxToRenderPerBatch={20}
          windowSize={5}
          removeClippedSubviews={true}
          ListEmptyComponent={
            <View style={[styles.center, { paddingTop: 60 }]}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>No tracks</Text>
            </View>
          }
        />
      ) : tab === "albums" ? (
        <FlatList
          data={albums ?? []}
          keyExtractor={(item) => item.ID}
          renderItem={renderAlbum}
          numColumns={NUM_ALBUM_COLS}
          contentContainerStyle={{ padding: 16, gap: 16 }}
          columnWrapperStyle={{ gap: 16 }}
          ListEmptyComponent={
            <View style={[styles.center, { paddingTop: 60 }]}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>No albums</Text>
            </View>
          }
        />
      ) : (
        <FlatList
          data={artists ?? []}
          keyExtractor={(item) => item.ID}
          renderItem={renderArtist}
          numColumns={NUM_ALBUM_COLS}
          contentContainerStyle={{ padding: 16, gap: 16 }}
          columnWrapperStyle={{ gap: 16 }}
          ListEmptyComponent={
            <View style={[styles.center, { paddingTop: 60 }]}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>No artists</Text>
            </View>
          }
        />
      )}

      {/* Queue modal */}
      <Modal visible={showQueue} transparent animationType="slide" onRequestClose={() => setShowQueue(false)}>
        <Pressable style={styles.queueBackdrop} onPress={() => setShowQueue(false)} />
        <View style={[styles.queueSheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 12 }}>Queue</Text>
          <FlatList
            data={queue}
            keyExtractor={(item, i) => `${item.id}-${i}`}
            style={{ maxHeight: 400 }}
            renderItem={({ item, index }) => (
              <Pressable
                onPress={() => playTrack(index)}
                style={[styles.queueRow, { backgroundColor: index === currentIndex ? theme.colors.surfaceRaised : "transparent", borderBottomColor: theme.colors.border }]}
              >
                <View style={{ flex: 1 }}>
                  <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: index === currentIndex ? "700" : "400" }} numberOfLines={1}>
                    {index === currentIndex ? "\u25B6 " : ""}{item.title}
                  </Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.artist}</Text>
                </View>
                <Pressable onPress={() => removeFromQueue(index)} hitSlop={8}>
                  <Ionicons name="close" size={16} color={theme.colors.textMuted} />
                </Pressable>
              </Pressable>
            )}
            ListEmptyComponent={<Text style={{ color: theme.colors.textMuted, textAlign: "center", paddingVertical: 24 }}>Queue empty</Text>}
          />
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 28, fontWeight: "700", marginBottom: 4 },
  libChip: { paddingHorizontal: 14, paddingVertical: 7 },
  tabBar: { flexDirection: "row", padding: 3 },
  tabButton: { flex: 1, paddingVertical: 7, alignItems: "center" },
  trackRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  trackIndex: { width: 28, alignItems: "center" },
  trackInfo: { flex: 1, paddingHorizontal: 10 },
  trackTitle: { fontSize: 14, fontWeight: "500" },
  albumCard: { overflow: "hidden" },
  albumTitle: { fontSize: 13, fontWeight: "600" },
  artPlaceholder: { justifyContent: "center", alignItems: "center" },
  albumsGrid: { flexDirection: "row", flexWrap: "wrap", gap: 16 },
  artistCard: { alignItems: "center", paddingVertical: 16, paddingHorizontal: 8 },
  artistAvatar: { borderRadius: 999, justifyContent: "center", alignItems: "center", marginBottom: 8 },
  artistName: { fontSize: 13, fontWeight: "600", textAlign: "center", marginBottom: 2 },
  queueBackdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: "rgba(0,0,0,0.5)" },
  queueSheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  queueRow: { flexDirection: "row", alignItems: "center", paddingVertical: 10, paddingHorizontal: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderRadius: 6 },
});
