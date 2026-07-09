import { useState, useCallback } from "react";
import {
  View,
  Text,
  TextInput,
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
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { Skeleton } from "../../../src/components/Skeleton";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { useMusicQueueStore, type QueueTrack } from "../../../src/stores/musicQueue";
import { AddToPlaylistSheet } from "../../../src/components/AddToPlaylistSheet";

interface LibraryItem {
  id: string;
  name: string;
  type: string;
}

// Field names match the JSON the gRPC gateway emits - protojson serializes
// proto field `track_number` as `trackNumber`, `id` as `id`, etc. The web app
// maps these to its own PascalCase domain types in a select(); we just read
// the wire shape directly here.
interface Track {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
  trackNumber: number;
  coverArtUrl?: string;
}

interface MusicAlbum {
  id: string;
  title: string;
  artist: string;
  year: number;
  trackCount: number;
  coverArtUrl?: string;
}

interface MusicArtist {
  id: string;
  name: string;
  trackCount: number;
  albumCount: number;
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

function useMusicTracks(libraryID: string | undefined, search?: string) {
  const q = (search ?? "").trim();
  return useQuery({
    queryKey: ["music", "tracks", libraryID, q],
    // A non-empty search hits the full-text index (Meilisearch, with SQL
    // fallback) server-side for typo tolerance; otherwise the plain listing.
    queryFn: () => eveyFetch<{ tracks: Track[] }>({
      url: q
        ? `/api/v1/music/tracks?library_id=${libraryID}&search=${encodeURIComponent(q)}`
        : `/api/v1/music/tracks?library_id=${libraryID}&limit=200`,
      method: "GET",
    }),
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
  const { t } = useTranslation();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();

  const [tab, setTab] = useState<Tab>("tracks");
  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [showQueue, setShowQueue] = useState(false);
  const [playlistTrackId, setPlaylistTrackId] = useState<string | null>(null);
  const [trackSearch, setTrackSearch] = useState("");
  const debouncedTrackSearch = useDebounce(trackSearch.trim(), 300);

  const queue = useMusicQueueStore((s) => s.queue);
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const playTrack = useMusicQueueStore((s) => s.playTrack);
  const removeFromQueue = useMusicQueueStore((s) => s.removeFromQueue);

  const { data: libraries, isLoading: libLoading } = useMusicLibraries();
  const activeLibraryID = selectedLibraryID ?? libraries?.[0]?.id;
  const { data: tracks, isLoading: tracksLoading } = useMusicTracks(activeLibraryID, debouncedTrackSearch);
  const { data: albums, isLoading: albumsLoading } = useMusicAlbums(activeLibraryID);
  const { data: artists, isLoading: artistsLoading } = useMusicArtists(activeLibraryID);

  const handlePlayTrack = useCallback((index: number) => {
    const allTracks = (tracks ?? []).map((t): QueueTrack => ({
      id: t.id,
      title: t.title,
      artist: t.artist,
      album: t.album,
      duration: t.duration,
    }));
    setQueue(allTracks, index);
  }, [tracks, setQueue]);

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
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{item.trackNumber || index + 1}</Text>
        </View>
        <View style={styles.trackInfo}>
          <Text style={[styles.trackTitle, { color: theme.colors.text }]} numberOfLines={1}>{item.title || "-"}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
            {[item.artist, item.album].filter(Boolean).join(" · ") || "-"}
          </Text>
        </View>
        {item.duration > 0 && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{formatDuration(item.duration)}</Text>
        )}
        <Pressable
          onPress={(e) => {
            e.stopPropagation();
            setPlaylistTrackId(item.id);
          }}
          hitSlop={8}
          style={styles.addBtn}
          accessibilityLabel="Add to playlist"
        >
          <Ionicons name="add-circle-outline" size={20} color={theme.colors.textMuted} />
        </Pressable>
      </TouchableOpacity>
    );},
    [theme, currentIndex, queue, handlePlayTrack],
  );

  const renderAlbum = useCallback(
    ({ item }: { item: MusicAlbum }) => {
      // Prefer the per-track cover URL the server already attached to the
      // album row; fall back to the album-level artwork endpoint. Resolve
      // relative paths against the active server (expo-image refuses
      // relative URIs, unlike browser <img>).
      const cover = item.coverArtUrl;
      const uri = cover
        ? (/^https?:/i.test(cover) ? cover : `${serverUrl ?? ""}${cover}`)
        : albumArtUri(item.id);
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
            <Text style={[styles.albumTitle, { color: theme.colors.text }]} numberOfLines={1}>{item.title || "-"}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>{item.artist || "-"}</Text>
            {item.year > 0 && (
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.year} · {item.trackCount} track{item.trackCount !== 1 ? "s" : ""}</Text>
            )}
          </View>
        </TouchableOpacity>
      );
    },
    [albumArtUri, authHeaders, theme, serverUrl],
  );

  // Hooks must run on every render in the same order - keep all useCallback
  // calls above the loading / empty early returns below. A previous version
  // declared renderArtist after the early returns, which crashed the screen
  // with "Rendered more hooks than during the previous render" once the
  // libraries query resolved.
  const renderArtist = useCallback(
    ({ item }: { item: MusicArtist }) => (
      <TouchableOpacity
        activeOpacity={0.7}
        style={[styles.artistCard, { width: ALBUM_ART_SIZE, backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      >
        <View style={[styles.artistAvatar, { width: ALBUM_ART_SIZE * 0.6, height: ALBUM_ART_SIZE * 0.6, backgroundColor: theme.colors.surfaceRaised }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 24, fontWeight: "700" }}>
            {item.name?.[0]?.toUpperCase() ?? "?"}
          </Text>
        </View>
        <Text style={[styles.artistName, { color: theme.colors.text }]} numberOfLines={2}>{item.name || "-"}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
          {item.albumCount} album{item.albumCount !== 1 ? "s" : ""} · {item.trackCount} track{item.trackCount !== 1 ? "s" : ""}
        </Text>
      </TouchableOpacity>
    ),
    [theme],
  );

  if (libLoading) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl, paddingHorizontal: theme.spacing.md }]}>{t("media.music.title")}</Text>
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
        <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{t("media.music.noMusicLibraries", "No music libraries")}</Text>
      </View>
    );
  }

  const isLoading = tab === "tracks" ? tracksLoading : tab === "albums" ? albumsLoading : artistsLoading;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl, paddingHorizontal: theme.spacing.md }]}>{t("media.music.title")}</Text>

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

      {/* Track search (Meilisearch-backed) */}
      {tab === "tracks" && (
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.sm }}>
          <View style={{ flexDirection: "row", alignItems: "center", backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm, paddingHorizontal: 10 }}>
            <Ionicons name="search-outline" size={16} color={theme.colors.textMuted} />
            <TextInput
              value={trackSearch}
              onChangeText={setTrackSearch}
              placeholder={t("media.search.placeholder")}
              placeholderTextColor={theme.colors.textMuted}
              autoCapitalize="none"
              autoCorrect={false}
              returnKeyType="search"
              style={{ flex: 1, color: theme.colors.text, fontSize: 14, paddingVertical: 8, marginLeft: 6 }}
            />
            {trackSearch.length > 0 && (
              <Pressable onPress={() => setTrackSearch("")} hitSlop={8} accessibilityLabel={t("media.search.clearSearch")}>
                <Ionicons name="close-circle" size={16} color={theme.colors.textMuted} />
              </Pressable>
            )}
          </View>
        </View>
      )}

      {/* Queue button */}
      {queue.length > 0 && (
        <Pressable
          onPress={() => setShowQueue(true)}
          style={{ flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: theme.spacing.md, paddingVertical: 6 }}
        >
          <Ionicons name="list" size={16} color={theme.colors.accent} />
          <Text style={{ color: theme.colors.accent, fontSize: 13 }}>{t("media.music.queue", "Queue")} ({queue.length})</Text>
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
          // Force a fresh FlatList per tab - RN's reconciler keeps the same
          // instance across the ternary branches and crashes on the
          // numColumns difference (1 ↔ NUM_ALBUM_COLS) otherwise.
          key="tracks"
          data={tracks ?? []}
          keyExtractor={(item) => item.id}
          renderItem={renderTrack}
          contentContainerStyle={{ paddingBottom: 32 }}
          initialNumToRender={20}
          maxToRenderPerBatch={20}
          windowSize={5}
          removeClippedSubviews={true}
          ListEmptyComponent={
            <View style={[styles.center, { paddingTop: 60 }]}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{t("media.music.noTracks")}</Text>
            </View>
          }
        />
      ) : tab === "albums" ? (
        <FlatList
          key="albums"
          data={albums ?? []}
          keyExtractor={(item) => item.id}
          renderItem={renderAlbum}
          numColumns={NUM_ALBUM_COLS}
          contentContainerStyle={{ padding: 16, gap: 16 }}
          columnWrapperStyle={{ gap: 16 }}
          ListEmptyComponent={
            <View style={[styles.center, { paddingTop: 60 }]}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{t("media.music.noAlbums")}</Text>
            </View>
          }
        />
      ) : (
        <FlatList
          key="artists"
          data={artists ?? []}
          keyExtractor={(item) => item.id}
          renderItem={renderArtist}
          numColumns={NUM_ALBUM_COLS}
          contentContainerStyle={{ padding: 16, gap: 16 }}
          columnWrapperStyle={{ gap: 16 }}
          ListEmptyComponent={
            <View style={[styles.center, { paddingTop: 60 }]}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>{t("media.music.noArtists")}</Text>
            </View>
          }
        />
      )}

      {/* Queue modal */}
      <Modal visible={showQueue} transparent animationType="slide" onRequestClose={() => setShowQueue(false)}>
        <Pressable style={styles.queueBackdrop} onPress={() => setShowQueue(false)} />
        <View style={[styles.queueSheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 12 }}>{t("media.music.queue", "Queue")}</Text>
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
            ListEmptyComponent={<Text style={{ color: theme.colors.textMuted, textAlign: "center", paddingVertical: 24 }}>{t("media.music.queueEmpty", "Queue empty")}</Text>}
          />
        </View>
      </Modal>

      {playlistTrackId && (
        <AddToPlaylistSheet
          visible={!!playlistTrackId}
          mediaItemId={playlistTrackId}
          mediaType="music"
          onClose={() => setPlaylistTrackId(null)}
        />
      )}
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
  addBtn: { padding: 6, marginLeft: 8 },
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
