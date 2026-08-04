import { useState, useMemo } from "react";
import { View, Text, ScrollView, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  useMusicServiceGetAlbums,
  useMusicServiceGetArtists,
  useMusicServiceListTracks,
} from "@evey/api";
import type { V1Library, V1MusicAlbum, V1MusicArtist, V1Track } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { ScreenState } from "../../../src/components/ScreenState";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { useMusicQueueStore, type QueueTrack } from "../../../src/stores/musicQueue";
import { musicAlbumHref, musicArtistHref } from "../../../src/lib/musicRoutes";
import { CARD_WIDTH_GRID } from "../../../src/lib/tvLayout";

const CARD_WIDTH = CARD_WIDTH_GRID;
const ART_SIZE = CARD_WIDTH;
const TRACKS_QUERY_LIMIT = 100;

function coverUri(serverUrl: string, coverArtUrl?: string): string | undefined {
  if (!coverArtUrl) return undefined;
  return /^https?:/i.test(coverArtUrl) ? coverArtUrl : `${serverUrl}${coverArtUrl}`;
}

function trackToQueueTrack(t: V1Track): QueueTrack {
  return {
    id: t.id ?? "",
    title: t.title ?? "",
    artist: t.artist ?? "",
    album: t.album ?? "",
    duration: t.duration ?? 0,
  };
}

function RowSeparator() {
  return <View style={{ width: 16 }} />;
}

// The music screen has three horizontal rows (artists, albums, tracks) that
// can each be empty independently. Only the first row that actually has
// items should claim the TV remote's initial focus - otherwise every row's
// first card fights for it simultaneously.
export function selectMusicFocusSection(sectionHasItems: boolean[]): number | null {
  const idx = sectionHasItems.findIndex(Boolean);
  return idx === -1 ? null : idx;
}

function useMusicLibraries() {
  return useLibraryServiceListLibraries({
    query: {
      select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC"),
      staleTime: 60_000,
    },
  });
}

function ArtistCard({
  artist,
  serverUrl,
  authHeaders,
  isFirst,
  libraryId,
}: {
  artist: V1MusicArtist;
  serverUrl: string;
  authHeaders?: { Authorization: string };
  isFirst?: boolean;
  libraryId: string;
}) {
  const theme = useTheme();
  const uri = coverUri(serverUrl, artist.coverArtUrl);
  return (
    <FocusableCard
      onPress={() => artist.id && router.push(musicArtistHref(artist.id, libraryId))}
      hasTVPreferredFocus={isFirst}
      style={styles.card}
      accessibilityLabel={artist.name}
    >
      <View style={[styles.artistArt, { backgroundColor: theme.colors.surface, borderRadius: ART_SIZE / 2 }]}>
        {uri ? (
          <Image
            source={{ uri, headers: authHeaders }}
            style={{ width: ART_SIZE, height: ART_SIZE, borderRadius: ART_SIZE / 2 }}
            contentFit="cover"
            transition={200}
            recyclingKey={artist.id}
          />
        ) : (
          <Text style={{ color: theme.colors.textMuted, fontSize: 40, fontWeight: "700" }}>
            {artist.name?.[0]?.toUpperCase() ?? "?"}
          </Text>
        )}
      </View>
      <Text style={[styles.cardTitle, { color: theme.colors.text }]} numberOfLines={1}>
        {artist.name || "-"}
      </Text>
      <Text style={[styles.cardSubtitle, { color: theme.colors.textMuted }]} numberOfLines={1}>
        {artist.albumCount ?? 0} · {artist.trackCount ?? 0}
      </Text>
    </FocusableCard>
  );
}

function AlbumCard({
  album,
  serverUrl,
  authHeaders,
  isFirst,
  libraryId,
}: {
  album: V1MusicAlbum;
  serverUrl: string;
  authHeaders?: { Authorization: string };
  isFirst?: boolean;
  libraryId: string;
}) {
  const theme = useTheme();
  const uri = coverUri(serverUrl, album.coverArtUrl);
  return (
    <FocusableCard
      onPress={() => album.id && router.push(musicAlbumHref(album.id, libraryId))}
      hasTVPreferredFocus={isFirst}
      style={styles.card}
      accessibilityLabel={album.title}
    >
      <View style={[styles.albumArt, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}>
        {uri ? (
          <Image
            source={{ uri, headers: authHeaders }}
            style={{ width: ART_SIZE, height: ART_SIZE, borderRadius: theme.radii.sm }}
            contentFit="cover"
            transition={200}
            recyclingKey={album.id}
          />
        ) : (
          <Text style={{ fontSize: 40 }}>{"♪"}</Text>
        )}
      </View>
      <Text style={[styles.cardTitle, { color: theme.colors.text }]} numberOfLines={1}>
        {album.title || "-"}
      </Text>
      <Text style={[styles.cardSubtitle, { color: theme.colors.textMuted }]} numberOfLines={1}>
        {album.artist || "-"}
      </Text>
    </FocusableCard>
  );
}

function TrackCard({
  track,
  serverUrl,
  authHeaders,
  isFirst,
  isPlaying,
  onPress,
}: {
  track: V1Track;
  serverUrl: string;
  authHeaders?: { Authorization: string };
  isFirst?: boolean;
  isPlaying?: boolean;
  onPress: () => void;
}) {
  const theme = useTheme();
  const uri = coverUri(serverUrl, track.coverArtUrl);
  return (
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      style={styles.card}
      accessibilityLabel={track.title}
    >
      <View
        style={[
          styles.albumArt,
          { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm },
          isPlaying && { borderColor: theme.colors.accent, borderWidth: 3 },
        ]}
      >
        {uri ? (
          <Image
            source={{ uri, headers: authHeaders }}
            style={{ width: ART_SIZE, height: ART_SIZE, borderRadius: theme.radii.sm }}
            contentFit="cover"
            transition={200}
            recyclingKey={track.id}
          />
        ) : (
          <Text style={{ fontSize: 40 }}>{"♪"}</Text>
        )}
      </View>
      <Text style={[styles.cardTitle, { color: isPlaying ? theme.colors.accent : theme.colors.text }]} numberOfLines={1}>
        {track.title || "-"}
      </Text>
      <Text style={[styles.cardSubtitle, { color: theme.colors.textMuted }]} numberOfLines={1}>
        {track.artist || "-"}
      </Text>
    </FocusableCard>
  );
}

function LibraryChip({
  library,
  active,
  onPress,
}: {
  library: V1Library;
  active: boolean;
  onPress: () => void;
}) {
  const theme = useTheme();
  return (
    <FocusableCard
      onPress={onPress}
      accessibilityLabel={library.name ?? ""}
      style={[
        styles.chip,
        { backgroundColor: active ? theme.colors.accent : theme.colors.surface, borderRadius: theme.radii.sm },
      ]}
    >
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 16, fontWeight: "600" }}>
        {library.name}
      </Text>
    </FocusableCard>
  );
}

function Section({
  title,
  loading,
  error,
  onRetry,
  empty,
  emptyLabel,
  children,
}: {
  title: string;
  loading: boolean;
  /** A row's own query failed - distinct from `empty`, which means the
   * request succeeded and simply found nothing. Conflating the two told the
   * viewer "no artists" when the artists request itself never answered. */
  error?: boolean;
  onRetry?: () => void;
  empty: boolean;
  emptyLabel: string;
  children: React.ReactNode;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <View style={styles.rowContainer}>
      <Text style={[styles.rowTitle, { color: theme.colors.text }]}>{title}</Text>
      {loading ? (
        <ActivityIndicator color={theme.colors.accent} style={{ marginLeft: 24 }} />
      ) : error ? (
        <View style={styles.rowError}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 16 }}>{t("common.loadError")}</Text>
          {onRetry ? (
            <FocusableCard onPress={onRetry} accessibilityLabel={t("common.retry")} style={styles.rowRetry}>
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>{t("common.retry")}</Text>
            </FocusableCard>
          ) : null}
        </View>
      ) : empty ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginLeft: 24 }}>{emptyLabel}</Text>
      ) : (
        children
      )}
    </View>
  );
}

export default function MusicScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);

  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const queue = useMusicQueueStore((s) => s.queue);
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const currentTrackId = currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex].id : undefined;

  const { data: libraries, isLoading: libLoading, isError: libError, refetch: refetchLibs } = useMusicLibraries();
  const activeLibraryID = selectedLibraryID ?? libraries?.[0]?.id;
  const libraryId = activeLibraryID ?? "";

  const {
    data: artists,
    isLoading: artistsLoading,
    isError: artistsError,
    refetch: refetchArtists,
  } = useMusicServiceGetArtists(
    { libraryId: activeLibraryID },
    { query: { enabled: !!activeLibraryID, select: (d) => d.artists ?? [] } },
  );
  const {
    data: albums,
    isLoading: albumsLoading,
    isError: albumsError,
    refetch: refetchAlbums,
  } = useMusicServiceGetAlbums(
    { libraryId: activeLibraryID },
    { query: { enabled: !!activeLibraryID, select: (d) => d.albums ?? [] } },
  );
  const {
    data: tracks,
    isLoading: tracksLoading,
    isError: tracksError,
    refetch: refetchTracks,
  } = useMusicServiceListTracks(
    { libraryId: activeLibraryID, limit: TRACKS_QUERY_LIMIT },
    { query: { enabled: !!activeLibraryID, select: (d) => d.tracks ?? [] } },
  );

  const focusSection = useMemo(
    () =>
      selectMusicFocusSection([
        (artists ?? []).length > 0,
        (albums ?? []).length > 0,
        (tracks ?? []).length > 0,
      ]),
    [artists, albums, tracks],
  );

  function handlePlayTrack(index: number) {
    setQueue((tracks ?? []).map(trackToQueueTrack), index);
  }

  // Before this guard a failed library fetch fell straight through to the
  // "no music library" state below - telling the viewer their collection is
  // empty when the server simply did not answer.
  if (libLoading || libError) {
    return <ScreenState loading={libLoading} error={libError} onRetry={() => void refetchLibs()} />;
  }

  if (!libraries || libraries.length === 0) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("media.music.noLibraries")}</Text>
      </View>
    );
  }

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ paddingVertical: 32 }}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("media.music.title")}</Text>

      {libraries.length > 1 && (
        <View style={styles.libRow}>
          {libraries.map((lib) => (
            <LibraryChip
              key={lib.id}
              library={lib}
              active={lib.id === activeLibraryID}
              onPress={() => setSelectedLibraryID(lib.id)}
            />
          ))}
        </View>
      )}

      <Section
        title={t("media.music.artists")}
        loading={artistsLoading}
        error={artistsError}
        onRetry={() => void refetchArtists()}
        empty={(artists ?? []).length === 0}
        emptyLabel={t("media.music.noArtists")}
      >
        <FlatList
          horizontal
          data={artists ?? []}
          keyExtractor={(item) => item.id ?? ""}
          renderItem={({ item, index }) => (
            <ArtistCard
              artist={item}
              serverUrl={serverUrl}
              authHeaders={authHeaders}
              isFirst={index === 0 && focusSection === 0}
              libraryId={libraryId}
            />
          )}
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={{ paddingHorizontal: 24 }}
          ItemSeparatorComponent={RowSeparator}
          initialNumToRender={6}
        />
      </Section>

      <Section
        title={t("media.music.albums")}
        loading={albumsLoading}
        error={albumsError}
        onRetry={() => void refetchAlbums()}
        empty={(albums ?? []).length === 0}
        emptyLabel={t("media.music.noAlbums")}
      >
        <FlatList
          horizontal
          data={albums ?? []}
          keyExtractor={(item) => item.id ?? ""}
          renderItem={({ item, index }) => (
            <AlbumCard
              album={item}
              serverUrl={serverUrl}
              authHeaders={authHeaders}
              isFirst={index === 0 && focusSection === 1}
              libraryId={libraryId}
            />
          )}
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={{ paddingHorizontal: 24 }}
          ItemSeparatorComponent={RowSeparator}
          initialNumToRender={6}
        />
      </Section>

      <Section
        title={t("media.music.tracks")}
        loading={tracksLoading}
        error={tracksError}
        onRetry={() => void refetchTracks()}
        empty={(tracks ?? []).length === 0}
        emptyLabel={t("media.music.noTracks")}
      >
        <FlatList
          horizontal
          data={tracks ?? []}
          keyExtractor={(item) => item.id ?? ""}
          renderItem={({ item, index }) => (
            <TrackCard
              track={item}
              serverUrl={serverUrl}
              authHeaders={authHeaders}
              isFirst={index === 0 && focusSection === 2}
              isPlaying={!!item.id && item.id === currentTrackId}
              onPress={() => handlePlayTrack(index)}
            />
          )}
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={{ paddingHorizontal: 24 }}
          ItemSeparatorComponent={RowSeparator}
          initialNumToRender={6}
        />
      </Section>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, marginBottom: 16 },
  rowContainer: { marginBottom: 32 },
  rowTitle: { fontSize: 24, fontWeight: "700", paddingHorizontal: 24, marginBottom: 16 },
  rowError: { flexDirection: "row", alignItems: "center", gap: 16, marginLeft: 24 },
  rowRetry: { paddingHorizontal: 16, paddingVertical: 6 },
  card: { width: CARD_WIDTH },
  albumArt: { width: ART_SIZE, height: ART_SIZE, justifyContent: "center", alignItems: "center", overflow: "hidden" },
  artistArt: { width: ART_SIZE, height: ART_SIZE, justifyContent: "center", alignItems: "center", overflow: "hidden" },
  cardTitle: { fontSize: 16, fontWeight: "600", marginTop: 8 },
  cardSubtitle: { fontSize: 14, marginTop: 2 },
  libRow: { flexDirection: "row", gap: 16, paddingHorizontal: 24, marginBottom: 24 },
  chip: { paddingVertical: 10, paddingHorizontal: 20 },
});
