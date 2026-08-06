import { useState, useCallback, useMemo, useEffect } from "react";
import { formatTime } from "@evey/api";
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
  Alert,
} from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { useLocalSearchParams, router } from "expo-router";
import { eveyFetch, useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { Skeleton } from "../../../src/components/Skeleton";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { useMusicQueueStore, type QueueTrack } from "../../../src/stores/musicQueue";
import { AddToPlaylistSheet } from "../../../src/components/AddToPlaylistSheet";
import { startDownload, getDownloadPath, deleteDownload, downloadErrorMessageKey } from "../../../src/lib/downloads";
import { isCurrentQueueTrack } from "../../../src/lib/currentTrack";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { buildDownloadAuthContext } from "../../../src/lib/downloadContext";
import { confirmDownloadOnMobileData } from "../../../src/lib/mobileDataGate";
import { toast, useAuthStore } from "@evey/store";
import { TrackArt } from "../../../src/components/music/TrackArt";
import { MusicPlaylistsTab } from "../../../src/components/music/MusicPlaylistsTab";
import type { Track } from "../../../src/components/music/musicTypes";
import { shuffleArray } from "../../../src/lib/shuffleArray";
import { trackListMusicSource, restoreMusicFilterFromParams } from "../../../src/components/music/musicSource";
import { averageAlbumProgress } from "../../../src/components/music/albumDownloadProgress";

interface LibraryItem {
  id: string;
  name: string;
  type: string;
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

type Tab = "tracks" | "albums" | "artists" | "playlists";

const NUM_ALBUM_COLS = 2;
const screenWidth = Dimensions.get("window").width;
const ALBUM_ART_SIZE = (screenWidth - 16 * 3) / NUM_ALBUM_COLS;

function useMusicLibraries() {
  return useQuery({
    queryKey: ["libraries", "music"],
    queryFn: () => eveyFetch<{ libraries: LibraryItem[] }>({ url: "/api/v1/libraries", method: "GET" }),
    select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC"),
    staleTime: 60_000,
  });
}

function useMusicTracks(libraryID: string | undefined, search?: string, albumID?: string) {
  const q = (search ?? "").trim();
  return useQuery({
    queryKey: ["music", "tracks", libraryID, q, albumID],
    // A non-empty search hits the full-text index (Meilisearch, with SQL
    // fallback) server-side for typo tolerance; otherwise the plain listing.
    // When albumID is set the server returns that album's full track set and
    // ignores search/limit - callers filter client-side on top of it.
    queryFn: () => eveyFetch<{ tracks: Track[] }>({
      url: albumID
        ? `/api/v1/music/tracks?library_id=${libraryID}&album_id=${encodeURIComponent(albumID)}&limit=200`
        : q
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
  // Same active-profile resolution as the player screen (player/[id].tsx's
  // activeProfile) and the media detail screen - so a track download captures
  // the exact profile identity/restriction that getDownloadPath's offline
  // gate will later check. Tracks carry no age classification (see Track in
  // musicTypes.ts), so buildDownloadAuthContext below is only ever given the
  // profile identity for music downloads.
  const activeProfileId = useAuthStore((s) => s.profileId);
  const { data: currentUserForProfile } = useUserServiceGetCurrentUser();
  const { data: profilesRespForProfile } = useUserServiceListProfiles(
    currentUserForProfile?.id ?? "",
    { query: { enabled: !!currentUserForProfile?.id } } as never,
  );
  const activeProfile = useMemo(
    () =>
      profilesRespForProfile?.profiles?.find((p) => p.id === activeProfileId) ??
      profilesRespForProfile?.profiles?.find((p) => p.isDefault) ??
      profilesRespForProfile?.profiles?.[0],
    [profilesRespForProfile, activeProfileId],
  );

  const [tab, setTab] = useState<Tab>("tracks");
  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [showQueue, setShowQueue] = useState(false);
  const [playlistTrackId, setPlaylistTrackId] = useState<string | null>(null);
  const [trackSearch, setTrackSearch] = useState("");
  const debouncedTrackSearch = useDebounce(trackSearch.trim(), 300);
  // Per-track download progress: a key is only present while that track's
  // download is running (0..1 known total, NaN unknown) - absent means idle.
  const [trackDownloads, setTrackDownloads] = useState<Record<string, number>>({});
  // Per-album download progress, same idle/absent convention as trackDownloads.
  const [albumDownloads, setAlbumDownloads] = useState<Record<string, number>>({});
  // "Hard" filters set by tapping an album/artist card - shown as a
  // dismissible chip above the track list, mirroring the web app.
  const [albumFilter, setAlbumFilter] = useState<string | null>(null);
  const [albumFilterLabel, setAlbumFilterLabel] = useState("");
  const [artistFilter, setArtistFilter] = useState<string | null>(null);
  const [showTrackFilters, setShowTrackFilters] = useState(false);
  const [genreFilter, setGenreFilter] = useState("");
  const [yearFilter, setYearFilter] = useState(0);
  const [artistPickFilter, setArtistPickFilter] = useState("");
  // Target playlist to auto-open in the playlists tab, restored from
  // navigation params (see the restore effect below).
  const [playlistTargetId, setPlaylistTargetId] = useState<string | null>(null);

  // Restores the album/artist/playlist filter this screen was launched with -
  // fed by the mini-player's "back to source" link (and any deep link that
  // reuses the same param shape). A restored album/artist/playlist target
  // always wins over whatever filter was showing before navigating away.
  const navParams = useLocalSearchParams<{ tab?: string; album?: string; albumLabel?: string; artist?: string; playlistId?: string }>();
  useEffect(() => {
    const restored = restoreMusicFilterFromParams(navParams);
    if (!restored) return;
    setTab(restored.tab);
    setAlbumFilter(restored.albumFilter);
    setAlbumFilterLabel(restored.albumFilterLabel);
    setArtistFilter(restored.artistFilter);
    setPlaylistTargetId(restored.playlistId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [navParams.tab, navParams.album, navParams.albumLabel, navParams.artist, navParams.playlistId]);

  const queue = useMusicQueueStore((s) => s.queue);
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const playTrack = useMusicQueueStore((s) => s.playTrack);
  const removeFromQueue = useMusicQueueStore((s) => s.removeFromQueue);

  const { data: libraries, isLoading: libLoading, isError: libError, refetch: refetchLibs } = useMusicLibraries();
  const activeLibraryID = selectedLibraryID ?? libraries?.[0]?.id;
  const { data: tracks, isLoading: tracksLoading, isError: tracksError, refetch: refetchTracks } = useMusicTracks(
    activeLibraryID,
    albumFilter ? undefined : debouncedTrackSearch,
    albumFilter ?? undefined,
  );
  const { data: albums, isLoading: albumsLoading, isError: albumsError, refetch: refetchAlbums } = useMusicAlbums(activeLibraryID);
  const { data: artists, isLoading: artistsLoading, isError: artistsError, refetch: refetchArtists } = useMusicArtists(activeLibraryID);

  // Genre/year/artist chip options are derived from the loaded (album-scoped)
  // track set, before the genre/year/artist-picker filters below are applied.
  const genreOptions = useMemo(
    () => Array.from(new Set((tracks ?? []).map((t) => t.genre).filter((g): g is string => !!g))).sort((a, b) => a.localeCompare(b)),
    [tracks],
  );
  const yearOptions = useMemo(
    () => Array.from(new Set((tracks ?? []).map((t) => t.year).filter((y): y is number => !!y))).sort((a, b) => b - a),
    [tracks],
  );
  const artistOptions = useMemo(() => {
    const set = new Set<string>();
    (tracks ?? []).forEach((t) => { if (t.artist) set.add(t.artist); if (t.albumArtist) set.add(t.albumArtist); });
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [tracks]);

  // Client-side filter pipeline on top of the server-scoped track list: the
  // free-text search box (when an album is already selected server-side), the
  // hard artist-card filter, and the genre/year/artist-picker filters.
  const filteredTracks = useMemo(() => {
    let list = tracks ?? [];
    if (albumFilter && debouncedTrackSearch.trim()) {
      const q = debouncedTrackSearch.trim().toLowerCase();
      list = list.filter(
        (t) => t.title?.toLowerCase().includes(q) || t.artist?.toLowerCase().includes(q) || t.album?.toLowerCase().includes(q),
      );
    }
    if (artistFilter) {
      list = list.filter((t) => t.artist === artistFilter || t.albumArtist === artistFilter);
    }
    if (genreFilter) {
      list = list.filter((t) => t.genre === genreFilter);
    }
    if (yearFilter) {
      list = list.filter((t) => t.year === yearFilter);
    }
    if (artistPickFilter) {
      list = list.filter((t) => t.artist === artistPickFilter || t.albumArtist === artistPickFilter);
    }
    return list;
  }, [tracks, albumFilter, debouncedTrackSearch, artistFilter, genreFilter, yearFilter, artistPickFilter]);

  const hasExtraTrackFilter = !!genreFilter || !!yearFilter || !!artistPickFilter;

  const handlePlayTrack = useCallback((index: number) => {
    const allTracks = filteredTracks.map((t): QueueTrack => ({
      id: t.id,
      title: t.title,
      artist: t.artist,
      album: t.album,
      duration: t.duration,
    }));
    setQueue(allTracks, index, trackListMusicSource(albumFilter, albumFilterLabel, artistFilter));
  }, [filteredTracks, setQueue, albumFilter, albumFilterLabel, artistFilter]);

  // "Shuffle play" the whole (album/search-scoped) library listing - an
  // ephemeral randomized queue, not a saved playlist.
  const handleShufflePlay = useCallback(() => {
    const pool = tracks ?? [];
    if (pool.length === 0) return;
    const shuffled = shuffleArray(pool).map((t): QueueTrack => ({
      id: t.id,
      title: t.title,
      artist: t.artist,
      album: t.album,
      duration: t.duration,
    }));
    setQueue(shuffled, 0, trackListMusicSource(albumFilter, albumFilterLabel, artistFilter));
  }, [tracks, setQueue, albumFilter, albumFilterLabel, artistFilter]);

  // Tracks are single-file media items - same server endpoint and startDownload
  // path as a movie/episode download, just tagged "audio" for the extension.
  const handleDownloadTrack = useCallback(
    async (track: Track) => {
      if (trackDownloads[track.id] !== undefined) return;
      const existing = await getDownloadPath(track.id);
      if (existing) {
        // Same dialog wording as the Downloads screen: a tap on an
        // already-downloaded track offers removal instead of a dead-end toast.
        Alert.alert(
          t("downloads.deleteTitle", "Delete download"),
          t("downloads.deleteConfirm", "Remove \"{{title}}\" from this device?", { title: track.title || track.id }),
          [
            { text: t("common.cancel", "Cancel"), style: "cancel" },
            {
              text: t("common.delete", "Delete"),
              style: "destructive",
              onPress: async () => {
                await deleteDownload(track.id);
                toast.success(t("downloads.deleted", "Download removed"));
              },
            },
          ],
        );
        return;
      }
      if (!(await confirmDownloadOnMobileData(t))) return;
      setTrackDownloads((s) => ({ ...s, [track.id]: NaN }));
      toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
      try {
        await startDownload(
          track.id,
          track.title || track.id,
          (p) => setTrackDownloads((s) => ({ ...s, [track.id]: p })),
          {
            kind: "audio",
            // A track isn't a library media item - it mints through the
            // generic download-token endpoint and its own download route
            // (same ones the web app's mintDownloadToken/track download use).
            tokenUrl: `/api/v1/download-token/${encodeURIComponent(track.id)}`,
            downloadPath: `/api/v1/music/tracks/${encodeURIComponent(track.id)}/download`,
            // Track carries no rating field (music isn't age-classified in
            // this schema) - only the downloader's profile identity is captured.
            ...buildDownloadAuthContext(activeProfile, undefined),
          },
        );
        toast.success(t("media.detail.downloadComplete"));
      } catch (err) {
        toast.error(t(downloadErrorMessageKey(err)));
      } finally {
        setTrackDownloads((s) => {
          const next = { ...s };
          delete next[track.id];
          return next;
        });
      }
    },
    [trackDownloads, t, activeProfile],
  );

  // The server also exposes a whole-album .zip archive
  // (GET /api/v1/music/albums/download, used by the web app's downloadAlbum),
  // but the mobile downloader (downloads.ts) has no unzip step and its
  // persisted index maps one media id to one offline-playable file - a zip
  // can't be represented there. Downloading every track of the album
  // individually through the existing per-track mechanism gets to the same
  // practical result (the whole album available offline) without extending
  // that model.
  const handleDownloadAlbum = useCallback(
    async (album: MusicAlbum) => {
      if (albumDownloads[album.id] !== undefined) return;
      if (!activeLibraryID) return;
      if (!(await confirmDownloadOnMobileData(t))) return;
      setAlbumDownloads((s) => ({ ...s, [album.id]: 0 }));
      toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
      try {
        const { tracks: albumTracks } = await eveyFetch<{ tracks: Track[] }>({
          url: `/api/v1/music/tracks?library_id=${activeLibraryID}&album_id=${encodeURIComponent(album.id)}&limit=200`,
          method: "GET",
        });
        const perTrackProgress: Record<string, number> = {};
        let downloadedAny = false;
        for (const track of albumTracks ?? []) {
          const existing = await getDownloadPath(track.id);
          if (existing) { perTrackProgress[track.id] = 1; continue; }
          downloadedAny = true;
          await startDownload(
            track.id,
            track.title || track.id,
            (p) => {
              perTrackProgress[track.id] = p;
              setAlbumDownloads((s) => ({ ...s, [album.id]: averageAlbumProgress(Object.values(perTrackProgress)) }));
            },
            {
              kind: "audio",
              tokenUrl: `/api/v1/download-token/${encodeURIComponent(track.id)}`,
              downloadPath: `/api/v1/music/tracks/${encodeURIComponent(track.id)}/download`,
              ...buildDownloadAuthContext(activeProfile, undefined),
            },
          );
          perTrackProgress[track.id] = 1;
        }
        toast.success(downloadedAny ? t("media.detail.downloadComplete") : t("media.detail.alreadyDownloaded"));
      } catch (err) {
        toast.error(t(downloadErrorMessageKey(err)));
      } finally {
        setAlbumDownloads((s) => {
          const next = { ...s };
          delete next[album.id];
          return next;
        });
      }
    },
    [albumDownloads, activeLibraryID, t, activeProfile],
  );

  const albumArtUri = useCallback(
    (albumID: string) => (serverUrl ? `${serverUrl}/api/v1/music/albums/${albumID}/artwork` : undefined),
    [serverUrl],
  );

  const renderTrack = useCallback(
    ({ item, index }: { item: Track; index: number }) => {
      // By id, never by position: this list is filtered and the queue is not.
      // The queue panel further down still compares indices, and rightly so -
      // there the index IS the queue position.
      const isCurrentTrack = isCurrentQueueTrack(queue, currentIndex, item.id);
      return (
      <TouchableOpacity
        activeOpacity={0.7}
        onPress={() => handlePlayTrack(index)}
        style={[styles.trackRow, { borderBottomColor: theme.colors.surface, backgroundColor: isCurrentTrack ? theme.colors.surface : "transparent" }]}
      >
        <View style={styles.trackIndex}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{item.trackNumber || index + 1}</Text>
        </View>
        <View style={{ marginLeft: 4 }}>
          <TrackArt trackID={item.id} size={36} authHeaders={authHeaders} serverUrl={serverUrl} theme={theme} />
        </View>
        <View style={styles.trackInfo}>
          <Text style={[styles.trackTitle, { color: theme.colors.text }]} numberOfLines={1}>{item.title || "-"}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
            {[item.artist, item.album].filter(Boolean).join(" · ") || "-"}
          </Text>
        </View>
        {item.duration > 0 && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{formatTime(item.duration)}</Text>
        )}
        <Pressable
          onPress={(e) => {
            e.stopPropagation();
            void handleDownloadTrack(item);
          }}
          disabled={trackDownloads[item.id] !== undefined}
          hitSlop={8}
          style={styles.addBtn}
          accessibilityLabel={t("common.download") || "Download"}
        >
          {trackDownloads[item.id] !== undefined ? (
            Number.isFinite(trackDownloads[item.id]) ? (
              <Text style={{ color: theme.colors.accent, fontSize: 10, fontWeight: "700" }}>
                {Math.round(trackDownloads[item.id] * 100)}%
              </Text>
            ) : (
              <Ionicons name="cloud-download-outline" size={20} color={theme.colors.accent} />
            )
          ) : (
            <Ionicons name="download-outline" size={20} color={theme.colors.textMuted} />
          )}
        </Pressable>
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
    [theme, currentIndex, queue, handlePlayTrack, trackDownloads, handleDownloadTrack, t, authHeaders, serverUrl],
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
          onPress={() => {
            setAlbumFilter(item.id);
            setAlbumFilterLabel(item.title);
            setArtistFilter(null);
            setTab("tracks");
          }}
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
          <Pressable
            onPress={(e) => {
              e.stopPropagation();
              void handleDownloadAlbum(item);
            }}
            disabled={albumDownloads[item.id] !== undefined}
            hitSlop={8}
            style={[styles.albumDownloadBtn, { backgroundColor: theme.colors.surface }]}
            accessibilityLabel={t("media.music.downloadAlbum")}
          >
            {albumDownloads[item.id] !== undefined ? (
              Number.isFinite(albumDownloads[item.id]) ? (
                <Text style={{ color: theme.colors.accent, fontSize: 10, fontWeight: "700" }}>
                  {Math.round(albumDownloads[item.id] * 100)}%
                </Text>
              ) : (
                <Ionicons name="cloud-download-outline" size={18} color={theme.colors.accent} />
              )
            ) : (
              <Ionicons name="download-outline" size={18} color={theme.colors.textMuted} />
            )}
          </Pressable>
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
    [albumArtUri, authHeaders, theme, serverUrl, setTab, handleDownloadAlbum, albumDownloads, t],
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
        onPress={() => {
          setArtistFilter(item.name);
          setAlbumFilter(null);
          setAlbumFilterLabel("");
          setTab("tracks");
        }}
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
    [theme, setTab],
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

  const isLoading = tab === "tracks" ? tracksLoading : tab === "albums" ? albumsLoading : tab === "artists" ? artistsLoading : false;
  // Per tab, because each one has its own request: an album listing that failed
  // says nothing about the tracks tab next door. Without this the failed tab
  // simply rendered its empty state - "no albums" for a server that is down.
  const isError =
    libError ||
    (tab === "tracks" ? tracksError : tab === "albums" ? albumsError : tab === "artists" ? artistsError : false);
  const retryTab = useCallback(() => {
    if (libError) void refetchLibs();
    if (tab === "tracks") void refetchTracks();
    else if (tab === "albums") void refetchAlbums();
    else if (tab === "artists") void refetchArtists();
  }, [libError, tab, refetchLibs, refetchTracks, refetchAlbums, refetchArtists]);

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
              <Text style={{ color: activeLibraryID === lib.id ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                {lib.name}
              </Text>
            </TouchableOpacity>
          ))}
        </ScrollView>
      )}

      {/* Tab switcher */}
      <View style={[styles.tabBar, { marginHorizontal: theme.spacing.md, marginTop: theme.spacing.md, backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}>
        {(["tracks", "albums", "artists", "playlists"] as Tab[]).map((tabKey) => (
          <TouchableOpacity
            key={tabKey}
            activeOpacity={0.8}
            onPress={() => setTab(tabKey)}
            style={[
              styles.tabButton,
              {
                backgroundColor: tab === tabKey ? theme.colors.accent : "transparent",
                borderRadius: theme.radii.sm,
              },
            ]}
          >
            <Text style={{ color: tab === tabKey ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 14, fontWeight: "600" }}>
              {t(`media.music.${tabKey}`)}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      {/* Track search (Meilisearch-backed) + filters + shuffle */}
      {tab === "tracks" && (
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.sm }}>
          <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
            <View style={{ flex: 1, flexDirection: "row", alignItems: "center", backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm, paddingHorizontal: 10 }}>
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
            <TouchableOpacity
              activeOpacity={0.7}
              onPress={() => setShowTrackFilters((v) => !v)}
              accessibilityLabel={t("media.library.filters")}
              style={[
                styles.iconBtn,
                { backgroundColor: showTrackFilters || hasExtraTrackFilter ? theme.colors.accent : theme.colors.surface, borderRadius: theme.radii.sm },
              ]}
            >
              <Ionicons name="options-outline" size={18} color={showTrackFilters || hasExtraTrackFilter ? theme.colors.onAccent : theme.colors.textMuted} />
            </TouchableOpacity>
            {(tracks ?? []).length > 0 && (
              <TouchableOpacity
                activeOpacity={0.7}
                onPress={handleShufflePlay}
                testID="music-shuffle-play"
                accessibilityLabel={t("media.music.playRandom")}
                style={[styles.iconBtn, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}
              >
                <Ionicons name="shuffle" size={18} color={theme.colors.textMuted} />
              </TouchableOpacity>
            )}
          </View>

          {/* Hard filter chips - album/artist card taps */}
          {(albumFilter || artistFilter) && (
            <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 8 }}>
              {albumFilter && (
                <Pressable
                  onPress={() => { setAlbumFilter(null); setAlbumFilterLabel(""); }}
                  style={[styles.filterChip, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.lg }]}
                >
                  <Text style={{ color: theme.colors.onAccent, fontSize: 12 }}>{t("media.music.albums")}: {albumFilterLabel || albumFilter}</Text>
                  <Ionicons name="close" size={13} color={theme.colors.onAccent} />
                </Pressable>
              )}
              {artistFilter && (
                <Pressable
                  onPress={() => setArtistFilter(null)}
                  style={[styles.filterChip, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.lg }]}
                >
                  <Text style={{ color: theme.colors.onAccent, fontSize: 12 }}>{t("media.music.artists")}: {artistFilter}</Text>
                  <Ionicons name="close" size={13} color={theme.colors.onAccent} />
                </Pressable>
              )}
            </View>
          )}

          {/* Genre / year / artist filter panel */}
          {showTrackFilters && (
            <View style={[styles.filterPanel, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <Text style={[styles.filterLabel, { color: theme.colors.textMuted }]}>{t("media.music.genre")}</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginBottom: 8 }}>
                {genreOptions.map((g) => (
                  <TouchableOpacity
                    key={g}
                    activeOpacity={0.7}
                    onPress={() => setGenreFilter(genreFilter === g ? "" : g)}
                    style={[styles.chip, { backgroundColor: genreFilter === g ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                  >
                    <Text style={{ color: genreFilter === g ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>{g}</Text>
                  </TouchableOpacity>
                ))}
              </ScrollView>

              <Text style={[styles.filterLabel, { color: theme.colors.textMuted }]}>{t("media.music.year")}</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginBottom: 8 }}>
                {yearOptions.map((y) => (
                  <TouchableOpacity
                    key={y}
                    activeOpacity={0.7}
                    onPress={() => setYearFilter(yearFilter === y ? 0 : y)}
                    style={[styles.chip, { backgroundColor: yearFilter === y ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                  >
                    <Text style={{ color: yearFilter === y ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>{y}</Text>
                  </TouchableOpacity>
                ))}
              </ScrollView>

              <Text style={[styles.filterLabel, { color: theme.colors.textMuted }]}>{t("media.music.artists")}</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginBottom: 8 }}>
                {artistOptions.map((a) => (
                  <TouchableOpacity
                    key={a}
                    activeOpacity={0.7}
                    onPress={() => setArtistPickFilter(artistPickFilter === a ? "" : a)}
                    style={[styles.chip, { backgroundColor: artistPickFilter === a ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                  >
                    <Text style={{ color: artistPickFilter === a ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>{a}</Text>
                  </TouchableOpacity>
                ))}
              </ScrollView>

              {hasExtraTrackFilter && (
                <TouchableOpacity
                  activeOpacity={0.7}
                  onPress={() => { setGenreFilter(""); setYearFilter(0); setArtistPickFilter(""); }}
                  style={[styles.chip, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm, alignSelf: "flex-start" }]}
                >
                  <Text style={{ color: theme.colors.text, fontSize: 13 }}>{t("media.music.resetFilters")}</Text>
                </TouchableOpacity>
              )}
            </View>
          )}
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
      {isError ? (
        <LoadFailed inline onRetry={retryTab} />
      ) : isLoading ? (
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
          data={filteredTracks}
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
      ) : tab === "artists" ? (
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
      ) : (
        <MusicPlaylistsTab libraryTracks={tracks ?? []} initialSelectedId={playlistTargetId} libraryId={activeLibraryID} />
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
  albumDownloadBtn: {
    position: "absolute",
    top: 6,
    right: 6,
    width: 26,
    height: 26,
    borderRadius: 13,
    alignItems: "center",
    justifyContent: "center",
  },
  albumTitle: { fontSize: 13, fontWeight: "600" },
  artPlaceholder: { justifyContent: "center", alignItems: "center" },
  albumsGrid: { flexDirection: "row", flexWrap: "wrap", gap: 16 },
  artistCard: { alignItems: "center", paddingVertical: 16, paddingHorizontal: 8 },
  artistAvatar: { borderRadius: 999, justifyContent: "center", alignItems: "center", marginBottom: 8 },
  artistName: { fontSize: 13, fontWeight: "600", textAlign: "center", marginBottom: 2 },
  queueBackdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: "rgba(0,0,0,0.5)" },
  queueSheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  queueRow: { flexDirection: "row", alignItems: "center", paddingVertical: 10, paddingHorizontal: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderRadius: 6 },
  iconBtn: { width: 36, height: 36, alignItems: "center", justifyContent: "center" },
  filterChip: { flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: 12, paddingVertical: 6 },
  filterPanel: { marginTop: 8, padding: 12 },
  filterLabel: { fontSize: 12, fontWeight: "600", marginBottom: 6, textTransform: "uppercase" },
  chip: { paddingHorizontal: 12, paddingVertical: 6, marginRight: 8 },
});
