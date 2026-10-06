import { useEffect, useState, useCallback, useMemo, useRef } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  FlatList,
  ActivityIndicator,
  StyleSheet,
  Linking,
  Dimensions,
  Share,
  Alert,
} from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Skeleton } from "../../../src/components/Skeleton";
import { GenreDot } from "../../../src/components/GenreDot";
import { useLocalSearchParams, router, useFocusEffect } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  libraryServiceListMediaItems,
  useLibraryServiceGetSeriesAggregate,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceGetWatchHistory,
  WATCH_HISTORY_OVERLAY_LIMIT,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  useShareServiceCreateShareLink,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useAuthStore, useDemoStore } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { certInfoForLocale } from "../../../src/lib/certifications";
import { useIsFavorite, useToggleFavorite } from "../../../src/hooks/useFavorite";
import { startDownload, getDownloadPath, deleteDownload, downloadErrorMessageKey, subscribeToProgress, type ProgressBytes } from "../../../src/lib/downloads";
import { MediaLoadFailed } from "../../../src/components/media/MediaLoadFailed";
import { downloadStageLabel } from "../../../src/lib/downloadStageLabel";
import { chooseDownloadQuality } from "../../../src/lib/downloadQualityGate";
import { startReducedDownload, reducedDownloadMessage } from "../../../src/lib/reducedDownload";
import { confirmDownloadOnMobileData } from "../../../src/lib/mobileDataGate";
import { seriesDownloadTargets, type EpisodeTarget } from "../../../src/lib/seriesDownloadTargets";
import { audioCodecsOf } from "../../../src/lib/playlistDownload";
import { formatBytes, formatDurationMs, formatTimeMs, itemLabel, seasonLabel, trackPickParams, type TrackPick } from "@evey/api";
import { TrackPickChips } from "../../../src/components/TrackPickChips";
import { buildDownloadAuthContext } from "../../../src/lib/downloadContext";
import { toast } from "@evey/store";
import { AddToPlaylistSheet } from "../../../src/components/AddToPlaylistSheet";
import { BulkAddToCollectionSheet } from "../../../src/components/library/BulkAddSheets";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";
import { MediaHeroBackdrop } from "../../../src/components/media/MediaHeroBackdrop";
import { backdropWidthForViewport } from "../../../src/components/media/backdropWidth";
import { hasMediaFile } from "../../../src/components/media/mediaFile";
import { buildPublicShareUrl } from "../../../src/components/media/shareLink";
import { MetadataEditSheet } from "../../../src/components/media/MetadataEditSheet";
import { PlayOnSheet } from "../../../src/components/media/PlayOnSheet";
import { SagaSection } from "../../../src/components/media/SagaSection";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";
import { RESUME_PROGRESS_QUERY, invalidateWatchedState, useSeriesPlayTarget } from "@evey/client";
import { useRefetchOnReturn } from "@evey/client/native/useRefetchOnReturn";
import { StarIcons } from "../../../src/components/media/StarIcons";
import { personKeyExtractor, renderCastItem } from "../../../src/components/media/CastCard";
import { EpisodeRow, type EpisodeProgress } from "../../../src/components/media/EpisodeRow";
import { TrailerButton } from "../../../src/components/media/TrailerButton";
import { SimilarSection } from "../../../src/components/media/SimilarSection";
import { FileInfoSection } from "../../../src/components/media/FileInfoSection";
import { useMyThumbRating, useSetThumbRating } from "../../../src/hooks/useThumbRating";

const POSTER_WIDTH = 100;
const POSTER_HEIGHT = POSTER_WIDTH * 1.5;
const EPISODE_PAGE_SIZE = 20;

function isSeriesType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SERIES";
}

function isSeasonType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SEASON";
}

function isEpisodeType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_EPISODE";
}

function formatEpisodeLabel(seasonIndex?: number, episodeIndex?: number): string | null {
  const s = seasonIndex ?? 0;
  const e = episodeIndex ?? 0;
  if (s > 0 && e > 0) return `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}`;
  if (e > 0) return `E${String(e).padStart(2, "0")}`;
  return null;
}

// Per-source critic ratings shown in the hero (web parity: MediaDetailHero).
const RATING_SOURCE_LABELS: Record<string, string> = { imdb: "IMDb", rt: "RT", metacritic: "Metacritic" };
function formatRating(value: number, maxValue: number): string {
  // 100-scale sources (RT, Metacritic) read as a score; 10-scale (IMDb) keeps a decimal.
  return maxValue === 100 ? `${Math.round(value)}` : value.toFixed(1);
}

// Aggregate runtime of a whole series, e.g. "2d 5h" (web parity:
// formatSeriesRuntime). Empty when there is nothing to sum.
function formatSeriesRuntime(totalMs?: string, episodeCount?: number): string {
  const ms = totalMs ? parseInt(totalMs, 10) : 0;
  const eps = episodeCount ?? 0;
  if (eps === 0 || isNaN(ms) || ms <= 0) return "";
  const totalMin = Math.floor(ms / 60000);
  const days = Math.floor(totalMin / (60 * 24));
  const h = Math.floor((totalMin % (60 * 24)) / 60);
  const m = totalMin % 60;
  if (days > 0) return `${days}d ${h}h`;
  if (h > 0) return m > 0 ? `${h}h ${m}min` : `${h}h`;
  return `${m}min`;
}

export default function MediaDetailScreen() {
  const { id, edit: editParam } = useLocalSearchParams<{ id: string; edit?: string }>();
  const { t, i18n } = useTranslation();
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const insets = useSafeAreaInsets();
  const [screenW, setScreenW] = useState(() => Dimensions.get("window").width);
  const [screenH, setScreenH] = useState(() => Dimensions.get("window").height);
  const dimsTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    const sub = Dimensions.addEventListener("change", ({ window }) => {
      if (dimsTimer.current) clearTimeout(dimsTimer.current);
      dimsTimer.current = setTimeout(() => {
        setScreenW(window.width);
        setScreenH(window.height);
      }, 300);
    });
    return () => { sub.remove(); if (dimsTimer.current) clearTimeout(dimsTimer.current); };
  }, []);
  const isLandscape = screenW > screenH;
  const heroHeight = isLandscape ? screenH * 0.55 : screenH * 0.4;
  // Dimensions().scale IS the device pixel ratio for the window - reused here
  // instead of importing PixelRatio separately since Dimensions is already
  // pulled in above. Falls back to 1x if a test double's Dimensions mock
  // omits it.
  const pixelRatio = Dimensions.get("window").scale || 1;
  const backdropWidth = backdropWidthForViewport(screenW, pixelRatio);
  // The poster is a 100dp thumbnail: ask for its device-pixel size, not the
  // full-resolution original (audit perf 2026-08-25).
  const posterWidth = Math.round(POSTER_WIDTH * pixelRatio);
  const [selectedSeason, setSelectedSeason] = useState<string | null>(null);
  // Episodes render inside the screen's single outer ScrollView (see below), so a
  // nested FlatList would trip "VirtualizedLists should never be nested inside
  // plain ScrollViews" and gain nothing since it can't own its own scroll. Instead
  // cap the initial render and let the user page in more with a button, which keeps
  // a 100-episode season from mounting 100 rows (with progress queries each) at once.
  const [visibleEpisodeCount, setVisibleEpisodeCount] = useState(EPISODE_PAGE_SIZE);
  useEffect(() => { setVisibleEpisodeCount(EPISODE_PAGE_SIZE); }, [selectedSeason]);

  const { data: media, isLoading, isError, error: mediaError, refetch } = useLibraryServiceGetMediaItem(id ?? "");

  const { data: currentUser } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(currentUser?.id ?? "", { query: { enabled: !!currentUser?.id } } as never);
  const profileRegion = (profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0])?.region;

  const { data: watchProgress, refetch: refetchProgress } = usePlaybackServiceGetWatchProgress(id ?? "", undefined, {
    query: { ...RESUME_PROGRESS_QUERY, enabled: !!id, retry: false },
  });
  // The player stacks on top of this screen and autoplay moves on from the
  // episode it opened with: back here, the Resume label must be re-read.
  useRefetchOnReturn(refetchProgress);
  const profileId = useAuthStore((s) => s.profileId);
  const demo = useDemoStore((s) => s.demoMode);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  // Same active-profile resolution as the player screen (player/[id].tsx's
  // activeProfile) - reused here so a download captures the exact profile
  // identity/restriction that getDownloadPath's offline gate will later check.
  const activeProfile = useMemo(
    () =>
      profilesResp?.profiles?.find((p) => p.id === profileId) ??
      profilesResp?.profiles?.find((p) => p.isDefault) ??
      profilesResp?.profiles?.[0],
    [profilesResp, profileId],
  );
  // Admin-only: mint a PUBLIC download link and hand it off via the native
  // share sheet - the mobile equivalent of the web's clipboard copy (there is
  // no clipboard API in this app; SyncPlayBar's room-code "copy" uses the same
  // Share.share() trick). The toast fires as soon as the link is ready, same
  // as the web's instant "copied" feedback, independent of what the user does
  // with the share sheet afterwards.
  const createShareLink = useShareServiceCreateShareLink({
    mutation: {
      onSuccess: async (res) => {
        const url = buildPublicShareUrl(serverUrl, res?.url);
        if (!url) {
          toast.error(t("media.library.shareFailed"));
          return;
        }
        toast.success(t("media.library.shareLinkCopied"));
        try {
          await Share.share({ message: url });
        } catch {
          // user dismissed the share sheet
        }
      },
      onError: (err: Error) => toast.error(err?.message || t("media.library.shareFailed")),
    },
  });
  const qc = useQueryClient();

  const { data: isFavorite } = useIsFavorite(id);
  const toggleFavorite = useToggleFavorite(id);
  const { data: myRating = 0, isError: ratingUnknown } = useMyThumbRating(id);
  const setThumbRating = useSetThumbRating(id);
  // A failed read leaves the thumbs on NEUTRAL, which is a statement ("no
  // opinion"), not an absence - and tapping from there writes it over an
  // opinion the viewer was never shown. The server stopped pretending on
  // 2026-08-24; the screens are catching up. Same guard on web and TV.
  const thumbsBusy = setThumbRating.isPending || ratingUnknown;


  const [playlistSheetOpen, setPlaylistSheetOpen] = useState(false);
  const [playOnSheetOpen, setPlayOnSheetOpen] = useState(false);
  const [collectionSheetOpen, setCollectionSheetOpen] = useState(false);
  const [metadataSheetOpen, setMetadataSheetOpen] = useState(false);
  // ?edit=1 (the library's long-press "Edit" on one title) opens the editor on
  // arrival, like the web card menu's "Edit metadata".
  useEffect(() => {
    if (editParam === "1" && isAdmin) setMetadataSheetOpen(true);
  }, [editParam, isAdmin]);
  const [watchedOverride, setWatchedOverride] = useState<boolean | null>(null);
  const watched = watchedOverride ?? (watchProgress?.played === true);

  const { needsPin, profileId: gateProfileId } = useParentalGate(media?.officialRating);
  const [showPinModal, setShowPinModal] = useState(false);
  const [pendingPlayTarget, setPendingPlayTarget] = useState<{ id: string; restart: boolean } | null>(null);
  // Audio/subtitle chosen before playback; empty = the player decides.
  const [trackPick, setTrackPick] = useState<TrackPick>({});
  // null = idle, 0..1 = downloading with known total, NaN = downloading
  // with unknown total (server omits Content-Length).
  const [downloadProgress, setDownloadProgress] = useState<number | null>(null);
  // Raw byte counter shown next to the percent: the integer percent of a
  // multi-GB file reads "0%" for minutes on a slow link, and the moving MB
  // figure is what tells that apart from a stuck download.
  const [downloadBytes, setDownloadBytes] = useState<ProgressBytes | null>(null);
  // Per-episode progress: a key is only present while that episode's
  // download is running (0..1 known total, NaN unknown) - absent means idle,
  // matching EpisodeRow's `downloadProgress === undefined` idle check.
  const [episodeDownloads, setEpisodeDownloads] = useState<Record<string, number>>({});
  // Mirror of episodeDownloads for the in-flight guard in downloadEpisodeFile,
  // so that callback can stay identity-stable across progress ticks.
  const episodeDownloadsRef = useRef(episodeDownloads);
  episodeDownloadsRef.current = episodeDownloads;
  const [bulkDownload, setBulkDownload] = useState<{ done: number; total: number } | null>(null);

  // The shared list of watched-sensitive keys: the hand-rolled one here aimed at
  // a "/api/v1/activity/popular-media" key no query uses, and missed Up Next
  // and Continue Watching.
  const invalidateWatched = useCallback(() => {
    if (id) void qc.invalidateQueries({ queryKey: [`/api/v1/playback/${id}/progress`] });
    invalidateWatchedState(qc);
  }, [id, qc]);

  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(true); invalidateWatched(); },
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(false); invalidateWatched(); },
    },
  });

  const isSeries = isSeriesType(media?.type);
  const isSeason = isSeasonType(media?.type);
  const isEpisode = isEpisodeType(media?.type);

  // Aggregate runtime + episode count for a series (web parity): the series
  // item's own durationMs is empty, so without this a series showed no runtime.
  const { data: seriesAggregate } = useLibraryServiceGetSeriesAggregate(
    id ?? "",
    { query: { enabled: !!id && isSeries } } as never,
  );

  const { data: parentItem } = useLibraryServiceGetMediaItem(
    media?.parentId ?? "",
    { query: { enabled: !!media?.parentId && isEpisode } } as never,
  );

  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: id, "pagination.pageSize": 100 },
    { query: { enabled: !!(id && (isSeries || isSeason)) } },
  );

  const children = useMemo(() => childrenData?.items ?? [], [childrenData?.items]);

  // Navigating between two series reuses this screen (Expo Router re-renders
  // rather than remounting), so reset the selected season when the media id
  // changes - otherwise the guard below keeps the PREVIOUS series' season and
  // the episode query loads the wrong (or empty) episode list.
  useEffect(() => { setSelectedSeason(null); }, [id]);

  useEffect(() => {
    if (isSeries && children.length > 0 && !selectedSeason) {
      const sorted = [...children].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
      setSelectedSeason(sorted[0]?.id ?? null);
    }
  }, [isSeries, children, selectedSeason]);

  const { data: episodesData } = useLibraryServiceListMediaItems(
      // 500, not 100: episodes have no server-side sort key, so this list is
      // re-sorted by index on the device and a truncated page would put the
      // wrong episode first (the server answers in sort_title order).
    { parentId: selectedSeason ?? "", "pagination.pageSize": 500 },
    { query: { enabled: !!(selectedSeason && isSeries) } },
  );

  const episodes = useMemo(
    () =>
      [...(isSeason ? children : (episodesData?.items ?? []))].sort(
        (a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0),
      ),
    [isSeason, children, episodesData?.items],
  );
  const seasons = useMemo(
    () => (isSeries ? [...children].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0)) : []),
    [isSeries, children],
  );

  // One watch-history fetch feeds every EpisodeRow's badge/progress bar -
  // same source, shape and limits as library/[libraryId].tsx and
  // useWatchedMap. Replaces one GetWatchProgress per row (audit 25-08 H3:
  // a season open fired a 20-request burst on the weak-NAS target).
  const { data: episodeHistory } = usePlaybackServiceGetWatchHistory(
    { limit: WATCH_HISTORY_OVERLAY_LIMIT },
    { query: { staleTime: 30_000, enabled: isSeries || isSeason } },
  );
  const episodeProgressMap = useMemo(() => {
    const map = new Map<string, EpisodeProgress>();
    for (const w of episodeHistory?.items ?? []) {
      if (!w.mediaItemId) continue;
      map.set(w.mediaItemId, { percent: w.percentComplete ?? 0, played: w.played === true });
    }
    return map;
  }, [episodeHistory]);

  // Downloads a single episode (single-file, same server endpoint as a
  // movie). Shared by the per-episode button and the "download all" loop
  // below; `silent` skips the per-item toasts during a bulk run so the user
  // gets one start/finish toast for the whole batch instead of N of them.
  // Tapping download on an already-downloaded item offers removal instead of
  // the old dead-end "already downloaded" toast - same dialog wording as the
  // Downloads screen so both paths read as one feature.
  const confirmDeleteDownload = useCallback((mediaItemId: string, title: string) => {
    Alert.alert(
      t("downloads.deleteTitle", "Delete download"),
      t("downloads.deleteConfirm", "Remove \"{{title}}\" from this device?", { title }),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        {
          text: t("common.delete", "Delete"),
          style: "destructive",
          onPress: async () => {
            await deleteDownload(mediaItemId);
            toast.success(t("downloads.deleted", "Download removed"));
          },
        },
      ],
    );
  }, [t]);

  const downloadEpisodeFile = useCallback(
    async (ep: V1MediaItem, opts?: { silent?: boolean; seasonNumber?: number }) => {
      if (!ep.id) return;
      // Read the in-flight set through a ref, not the state value: depending on
      // episodeDownloads would give this callback a new identity on every
      // progress tick, which in turn re-renders EVERY memoised EpisodeRow
      // instead of just the one that is downloading.
      if (episodeDownloadsRef.current[ep.id] !== undefined) return;
      const existing = await getDownloadPath(ep.id);
      if (existing) {
        // Bulk "download all" must stay quiet about episodes already on disk;
        // a direct tap on the row's button gets the delete dialog instead.
        if (!opts?.silent) confirmDeleteDownload(ep.id, ep.title ?? ep.id);
        return;
      }
      // A bulk download asked once for the whole batch.
      if (!opts?.silent && !(await confirmDownloadOnMobileData(t))) return;
      setEpisodeDownloads((s) => ({ ...s, [ep.id!]: NaN }));
      if (!opts?.silent) toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
      try {
        await startDownload(
          ep.id,
          ep.title ?? ep.id,
          (p) => { if (screenMountedRef.current) setEpisodeDownloads((s) => ({ ...s, [ep.id!]: p })); },
          {
            kind: "video",
            formatHint: ep.containerFormat,
            audioCodecs: audioCodecsOf(ep),
            // Captured here because offline there is no way to resolve an
            // episode's ancestors: this screen IS the series, and the picked
            // season carries the number.
            display: {
              seriesTitle: media?.title,
              seasonNumber: opts?.seasonNumber
                ?? (isSeason ? media?.indexNumber : children.find((c) => c.id === selectedSeason)?.indexNumber),
              episodeNumber: ep.indexNumber,
            },
            // An episode rarely carries its own classification - it inherits
            // the series' rating, which is what this screen's own parental
            // gate above (useParentalGate(media?.officialRating)) already
            // relies on.
            ...buildDownloadAuthContext(activeProfile, {
              officialRating: ep.officialRating ?? media?.officialRating,
              certificationsByCountry: ep.certificationsByCountry ?? media?.certificationsByCountry,
            }),
          },
        );
        if (!opts?.silent) toast.success(t("media.detail.downloadComplete"));
      } catch (err) {
        if (!opts?.silent) {
          toast.error(t(downloadErrorMessageKey(err)));
        }
        throw err;
      } finally {
        setEpisodeDownloads((s) => {
          const next = { ...s };
          delete next[ep.id!];
          return next;
        });
      }
    },
    [t, activeProfile, media, confirmDeleteDownload, children, selectedSeason, isSeason],
  );

  // Stable prop for EpisodeRow. Passing an inline arrow here made the memo()
  // around EpisodeRow inert: every "Show more" tap re-rendered the whole list
  // (280 rows across four taps on a 100-episode season) instead of only the
  // twenty newly revealed ones.
  const handleEpisodeDownload = useCallback(
    (ep: V1MediaItem) => { void downloadEpisodeFile(ep); },
    [downloadEpisodeFile],
  );


  // "Download all" for the currently shown episode list (the active season,
  // for both a season screen and a series screen). Sequenced one at a time -
  // not a Promise.all - so a 24-episode season doesn't fire 24 concurrent
  // background downloads at once.
  const runBulkDownload = useCallback(async (targets: EpisodeTarget[]) => {
    if (targets.length === 0) return;
    // Asked once for the whole batch, not once per episode: a season is the
    // heaviest thing this app downloads, and prompting per episode would be
    // twenty dialogs.
    if (!(await confirmDownloadOnMobileData(t))) return;
    setBulkDownload({ done: 0, total: targets.length });
    toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
    let done = 0;
    for (const { episode, seasonNumber } of targets) {
      try {
        await downloadEpisodeFile(episode, { silent: true, seasonNumber });
      } catch {
        // One failed episode must not stop the rest of the batch from queuing.
      }
      done += 1;
      setBulkDownload({ done, total: targets.length });
    }
    setBulkDownload(null);
    toast.success(t("media.detail.downloadComplete"));
  }, [downloadEpisodeFile, t]);

  const handleDownloadAllEpisodes = useCallback(async () => {
    if (bulkDownload) return;
    await runBulkDownload(episodes.filter((ep) => ep.id && hasMediaFile(ep)).map((episode) => ({ episode })));
  }, [episodes, runBulkDownload, bulkDownload]);

  // Every season of the series, not only the one on show.
  const handleDownloadSeries = useCallback(async () => {
    if (bulkDownload) return;
    setBulkDownload({ done: 0, total: 0 });
    let targets: EpisodeTarget[];
    try {
      targets = await seriesDownloadTargets(seasons, async (seasonId) =>
        (await libraryServiceListMediaItems({ parentId: seasonId, "pagination.pageSize": 500 })).items ?? []);
    } catch (err) {
      setBulkDownload(null);
      toast.error(t(downloadErrorMessageKey(err)));
      return;
    }
    setBulkDownload(null);
    await runBulkDownload(targets);
  }, [seasons, runBulkDownload, bulkDownload, t]);

  const people = useMemo(() => media?.people ?? [], [media?.people]);
  const actors = useMemo(
    () => people.filter((p) => p.type === "PERSON_TYPE_ACTOR" || p.type === "PERSON_TYPE_GUEST_STAR"),
    [people],
  );
  const directors = useMemo(() => people.filter((p) => p.type === "PERSON_TYPE_DIRECTOR"), [people]);
  const writers = useMemo(() => people.filter((p) => p.type === "PERSON_TYPE_WRITER"), [people]);
  const producers = useMemo(() => people.filter((p) => p.type === "PERSON_TYPE_PRODUCER"), [people]);
  const crew = useMemo(
    () => [...directors, ...writers, ...producers],
    [directors, writers, producers],
  );

  // A series plays where the profile is in it (the server's answer); the
  // first episode shown stays the fallback until that answer is in.
  const seriesTarget = useSeriesPlayTarget(isSeries ? id : undefined);
  const getPlayTarget = useCallback((): string | null => {
    if (!id) return null;
    if (isSeries && seriesTarget.episode?.id) return seriesTarget.episode.id;
    if (isSeries && episodes.length > 0) return episodes[0]!.id!;
    if (isSeason && episodes.length > 0) return episodes[0]!.id!;
    return id;
  }, [id, isSeries, isSeason, episodes, seriesTarget.episode?.id]);

  // A double-tap on a play control fires this callback twice before the
  // first push unmounts the screen, mounting two player screens stacked on
  // top of each other. Latch until the screen regains focus (leaving the
  // player and coming back).
  const navigatingRef = useRef(false);
  useFocusEffect(
    useCallback(() => {
      navigatingRef.current = false;
    }, []),
  );

  // `restart` travels all the way to the player, PIN prompt included: the
  // choice must survive the detour, or entering a PIN would silently resume
  // the thing the viewer asked to watch from the start.
  const openPlayer = useCallback((target: string, restart: boolean) => {
    if (navigatingRef.current) return;
    navigatingRef.current = true;
    // The track pick belongs to THIS file's streams: an episode launched from
    // a series page keeps the player's own resolution.
    const query = new URLSearchParams(trackPickParams(target === media?.id ? trackPick : undefined));
    if (restart) query.set("restart", "1");
    const q = query.toString();
    router.push(q ? `/(auth)/player/${target}?${q}` : `/(auth)/player/${target}`);
  }, [media?.id, trackPick]);

  const handlePlay = useCallback((restart = false) => {
    const target = getPlayTarget();
    if (!target) return;
    if (needsPin()) {
      setPendingPlayTarget({ id: target, restart });
      setShowPinModal(true);
      return;
    }
    openPlayer(target, restart);
  }, [getPlayTarget, needsPin, openPlayer]);

  // The download-progress callbacks below outlive this screen: startDownload
  // unsubscribes them when the DOWNLOAD settles, not when the screen unmounts,
  // so a download finishing minutes after navigation kept firing setState into
  // a dead screen for its whole duration (audit 2026-08-09). Gate them here.
  const screenMountedRef = useRef(true);
  useEffect(() => () => { screenMountedRef.current = false; }, []);

  // An episode tap is a PLAY action, so it takes the same PIN gate as the Play
  // button - the row used to router.push straight into the player, which has
  // no check of its own, leaving the gate on one button only. needsPin rides a
  // ref to keep this callback identity-stable (it is a prop of every memoised
  // EpisodeRow, see handleEpisodeDownload's comment).
  const needsPinRef = useRef(needsPin);
  needsPinRef.current = needsPin;
  const handleEpisodePlay = useCallback((ep: V1MediaItem) => {
    if (!ep.id) return;
    if (needsPinRef.current()) {
      setPendingPlayTarget({ id: ep.id, restart: false });
      setShowPinModal(true);
      return;
    }
    openPlayer(ep.id, false);
  }, [openPlayer]);

  const handlePinVerified = useCallback(() => {
    setShowPinModal(false);
    if (pendingPlayTarget) {
      openPlayer(pendingPlayTarget.id, pendingPlayTarget.restart);
      setPendingPlayTarget(null);
    }
  }, [pendingPlayTarget, openPlayer]);

  // The React Query client retries twice then gives up (except 401/403/404):
  // once exhausted, isLoading turns false but media stays undefined. Without an
  // error branch that left the most-visited screen stuck on the loading skeleton
  // forever, with no message or retry. Mirrors library/[libraryId].tsx.
  if (isError && !media) {
    // A 404 is not a failure to load, it is a title that left the library
    // (deleted, or re-imported under a new id) while something still linked
    // here - an old notification, a stale deep link. Retrying can never
    // succeed; the way out is back.
    const gone = (mediaError as { httpStatus?: number } | null)?.httpStatus === 404;
    // A downloaded title stays playable from here: the player prefers the
    // local file and needs nothing from this page.
    return (
      <MediaLoadFailed
        mediaId={id ?? ""}
        gone={gone}
        onRetry={() => void refetch()}
        onBack={() => (router.canGoBack() ? router.back() : router.replace("/(auth)/(tabs)"))}
        onPlayLocal={() => openPlayer(id ?? "", false)}
      />
    );
  }

  if (isLoading || !media) {
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
        <Skeleton width={screenW} height={heroHeight} borderRadius={0} />
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.md }}>
          <Skeleton width={screenW * 0.6} height={24} borderRadius={theme.radii.sm} />
          <Skeleton width={screenW * 0.4} height={14} borderRadius={theme.radii.sm} style={{ marginTop: theme.spacing.sm }} />
          <Skeleton width={screenW - theme.spacing.md * 2} height={44} borderRadius={theme.radii.md} style={{ marginTop: theme.spacing.md }} />
          <Skeleton width={screenW - theme.spacing.md * 2} height={60} borderRadius={theme.radii.sm} style={{ marginTop: theme.spacing.md }} />
        </View>
      </View>
    );
  }

  const resumeMs = watchProgress?.positionMs;
  const hasProgress = resumeMs && Number(resumeMs) > 0 && !watchProgress?.played;

  const mediaLabel = itemLabel(media, t);

  const trailers = media.trailers ?? [];
  const sortedTrailers = [...trailers].sort((a, b) => {
    if (a.official && !b.official) return -1;
    if (!a.official && b.official) return 1;
    return 0;
  }).slice(0, 3);

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ paddingLeft: insets.left, paddingRight: insets.right, paddingBottom: insets.bottom }}>
      {/* Hero backdrop */}
      <MediaHeroBackdrop
        uri={serverUrl && id ? artworkUrl(serverUrl, id, "backdrop", backdropWidth, media) : undefined}
        headers={authHeaders}
        height={heroHeight}
        onBack={() => router.back()}
        topInset={insets.top}
        leftInset={insets.left}
      />

      {/* Content overlapping hero */}
      <View style={{ paddingHorizontal: theme.spacing.md, marginTop: -POSTER_HEIGHT * 0.5, ...(isLandscape ? { flexDirection: "row", gap: theme.spacing.lg } : {}) }}>
        {/* Left column (landscape) / full width (portrait) */}
        <View style={isLandscape ? { flex: 3 } : {}}>
        {/* Poster + title row */}
        <View style={{ flexDirection: "row", gap: theme.spacing.md }}>
          {serverUrl && id ? (
            <Image
              source={{ uri: artworkUrl(serverUrl, id, "poster", posterWidth, media), headers: authHeaders }}
              style={{
                width: POSTER_WIDTH,
                height: POSTER_HEIGHT,
                borderRadius: theme.radii.md,
                backgroundColor: theme.colors.surface,
              }}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View style={{
              width: POSTER_WIDTH,
              height: POSTER_HEIGHT,
              borderRadius: theme.radii.md,
              backgroundColor: theme.colors.surface,
              justifyContent: "center",
              alignItems: "center",
            }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 28, fontWeight: "bold" }}>
                {mediaLabel[0]?.toUpperCase() ?? "?"}
              </Text>
            </View>
          )}

          <View style={{ flex: 1, justifyContent: "flex-end" }}>
            {isEpisode && (() => {
              const epLabel = formatEpisodeLabel(parentItem?.indexNumber, media.indexNumber);
              const seasonName = parentItem ? itemLabel(parentItem, t) : undefined;
              const label = [epLabel, seasonName].filter(Boolean).join(" · ");
              if (!label) return null;
              // Reached from the Resume row an episode lands here with no way
              // back up to its season; parentId is always present on an episode,
              // so make the SxEy · season line a tap-back to the season page.
              if (!media.parentId) {
                return (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 13, fontWeight: "600", marginBottom: 2 }}>
                    {label}
                  </Text>
                );
              }
              return (
                <Pressable
                  onPress={() => router.push(`/(auth)/media/${media.parentId}`)}
                  hitSlop={8}
                  accessibilityRole="button"
                  accessibilityLabel={seasonName ?? epLabel ?? undefined}
                  style={{ flexDirection: "row", alignItems: "center", gap: 3, marginBottom: 2 }}
                >
                  <Ionicons name="chevron-back" size={14} color={theme.colors.accent} />
                  <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
                    {label}
                  </Text>
                </Pressable>
              );
            })()}
            <Text style={{ color: theme.colors.text, fontSize: 22, fontWeight: "bold" }}>
              {mediaLabel}
            </Text>

            {/* Meta: year, duration, rating */}
            <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 6, marginTop: 4 }}>
              {media.year ? (
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{media.year}</Text>
              ) : null}
              {(() => {
                // Series: the whole-show aggregate ("2d 5h"); everything else:
                // this item's own runtime.
                const runtime = isSeries
                  ? formatSeriesRuntime(seriesAggregate?.totalDurationMs, seriesAggregate?.episodeCount)
                  : formatDurationMs(media.durationMs);
                return runtime
                  ? <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{runtime}</Text>
                  : null;
              })()}
              {isSeries && (seriesAggregate?.episodeCount ?? 0) > 0 ? (
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
                  {t("media.detail.episodeCount", { count: seriesAggregate!.episodeCount })}
                </Text>
              ) : null}
              {(() => {
                const info = certInfoForLocale(media.certificationsByCountry, i18n.language, media.officialRating, profileRegion);
                if (!info) return null;
                const body = t(`media.certBody.${info.country}`, { defaultValue: info.country });
                return (
                  <View style={{
                    borderWidth: 1,
                    borderColor: theme.colors.textMuted,
                    borderRadius: theme.radii.sm,
                    paddingHorizontal: 5,
                    paddingVertical: 1,
                  }}>
                    <Text style={{ color: theme.colors.text, fontSize: 11, fontWeight: "700" }}>
                      {body} {info.rating}
                    </Text>
                  </View>
                );
              })()}
            </View>

            {/* Community rating stars + per-source critic ratings (web parity) */}
            {(media.communityRating != null && media.communityRating > 0) ||
            (media.ratings ?? []).some((r) => r.source && RATING_SOURCE_LABELS[r.source] && (r.value ?? 0) > 0) ? (
              <View style={{ flexDirection: "row", alignItems: "center", flexWrap: "wrap", gap: 8, marginTop: 4 }}>
                {media.communityRating != null && media.communityRating > 0 ? (
                  <View style={{ flexDirection: "row", alignItems: "center", gap: 4 }}>
                    <StarIcons rating={media.communityRating} />
                    <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                      {media.communityRating.toFixed(1)}
                    </Text>
                  </View>
                ) : null}
                {(media.ratings ?? [])
                  .filter((r) => r.source && RATING_SOURCE_LABELS[r.source] && (r.value ?? 0) > 0)
                  .map((r) => (
                    <Text key={r.source} style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                      <Text style={{ fontWeight: "700" }}>{RATING_SOURCE_LABELS[r.source!]}</Text>{" "}
                      {formatRating(r.value ?? 0, r.maxValue ?? 10)}{r.source === "rt" ? "%" : ""}
                    </Text>
                  ))}
              </View>
            ) : null}

            {/* Provider badges: IMDb, TMDB */}
            <View style={{ flexDirection: "row", gap: 6, marginTop: 4 }}>
              {media.providerIds?.imdb ? (
                <Pressable onPress={() => Linking.openURL(`https://www.imdb.com/title/${media.providerIds!.imdb}/`)} accessible={true} accessibilityLabel="IMDb" accessibilityRole="link">
                  <View style={{ backgroundColor: "#f5c518", borderRadius: 3, paddingHorizontal: 5, paddingVertical: 2 }}>
                    <Text style={{ color: "#000", fontSize: 10, fontWeight: "800" }}>IMDb</Text>
                  </View>
                </Pressable>
              ) : null}
              {media.providerIds?.tmdb ? (
                <Pressable onPress={() => Linking.openURL(`https://www.themoviedb.org/${media.type === "MEDIA_ITEM_TYPE_MOVIE" ? "movie" : "tv"}/${media.providerIds!.tmdb}`)} accessible={true} accessibilityLabel="TMDB" accessibilityRole="link">
                  <View style={{ backgroundColor: "#01b4e4", borderRadius: 3, paddingHorizontal: 5, paddingVertical: 2 }}>
                    <Text style={{ color: "#fff", fontSize: 10, fontWeight: "800" }}>TMDB</Text>
                  </View>
                </Pressable>
              ) : null}
            </View>
          </View>
        </View>

        {/* Genres */}
        {media.genres && media.genres.length > 0 ? (
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: theme.spacing.sm }}>
            {media.genres.map((g) => (
              <View key={g} style={{
                flexDirection: "row",
                alignItems: "center",
                gap: 5,
                paddingHorizontal: 10,
                paddingVertical: 4,
                borderRadius: 16,
                backgroundColor: theme.colors.surface,
              }}>
                <GenreDot label={g} />
                <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "500" }}>{g}</Text>
              </View>
            ))}
          </View>
        ) : null}

        {/* One control, two halves: the wide one resumes, the narrow one starts
            over. The divider is what makes them read as a single button rather
            than two rival Play buttons. With nothing to resume the second half
            is absent and the button is whole again. */}
        <View style={{ flexDirection: "row", marginTop: theme.spacing.md }}>
          <Pressable
            onPress={() => handlePlay(false)}
            style={{
              flex: 1,
              backgroundColor: theme.colors.accent,
              borderTopLeftRadius: theme.radii.md,
              borderBottomLeftRadius: theme.radii.md,
              borderTopRightRadius: hasProgress ? 0 : theme.radii.md,
              borderBottomRightRadius: hasProgress ? 0 : theme.radii.md,
              paddingVertical: theme.spacing.sm + 4,
              flexDirection: "row",
              alignItems: "center",
              justifyContent: "center",
              gap: 8,
            }}
            accessible={true}
            accessibilityLabel={
              hasProgress
                ? `${t("media.detail.resume")} ${formatTimeMs(Number(resumeMs))}`
                : isSeries && seriesTarget.started ? t("media.detail.resume") : t("media.detail.play")
            }
            accessibilityRole="button"
          >
            <Ionicons name="play" size={18} color={theme.colors.onAccent} />
            <Text style={{ color: theme.colors.onAccent, fontWeight: "700", fontSize: 16 }}>
              {hasProgress
                ? `${t("media.detail.resume")} ${formatTimeMs(Number(resumeMs))}`
                : isSeries && seriesTarget.started ? t("media.detail.resume") : t("media.detail.play")}
            </Text>
          </Pressable>

          {hasProgress ? (
            <Pressable
              onPress={() => handlePlay(true)}
              style={{
                backgroundColor: theme.colors.accent,
                borderTopRightRadius: theme.radii.md,
                borderBottomRightRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm + 4,
                paddingHorizontal: theme.spacing.md,
                justifyContent: "center",
                borderLeftWidth: StyleSheet.hairlineWidth,
                borderLeftColor: theme.colors.onAccent + "55",
              }}
              accessible={true}
              accessibilityLabel={t("media.playFromStart", "Play from the beginning")}
              accessibilityRole="button"
            >
              <Ionicons name="refresh" size={20} color={theme.colors.onAccent} />
            </Pressable>
          ) : null}
        </View>

        {!isSeries && !isSeason && media?.streams ? (
          <TrackPickChips streams={media.streams} pick={trackPick} onChange={setTrackPick} />
        ) : null}

        {/* Action row: favorite, watched, playlist */}
        <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.md, flexWrap: "wrap" }}>
          {/* On the demo the favourites, ratings, watched marks, playlists and
              collections are the shared account's and the server refuses
              them: not offered there (the televisions do the same). */}
          {demo ? null : (
            <>
          <Pressable
            onPress={() => toggleFavorite.mutate()}
            disabled={toggleFavorite.isPending}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
              opacity: toggleFavorite.isPending ? 0.5 : 1,
            }}
            accessible={true}
            accessibilityLabel={isFavorite ? t("media.detail.favorited") : t("media.detail.favorite")}
            accessibilityRole="button"
          >
            <Ionicons
              name={isFavorite ? "heart" : "heart-outline"}
              size={20}
              color={isFavorite ? theme.colors.accent : theme.colors.text}
            />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {isFavorite ? t("media.detail.favorited") : t("media.detail.favorite")}
            </Text>
          </Pressable>

          <Pressable
            onPress={() => setThumbRating.mutate(myRating === 1 ? 0 : 1)}
            disabled={thumbsBusy}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: myRating === 1 ? theme.colors.success : theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
              opacity: thumbsBusy ? 0.5 : 1,
            }}
            accessible={true}
            accessibilityLabel={myRating === 1 ? t("media.rating.removeLike") : t("media.rating.like")}
            accessibilityRole="button"
          >
            <Ionicons
              name={myRating === 1 ? "thumbs-up" : "thumbs-up-outline"}
              size={20}
              color={myRating === 1 ? theme.colors.onAccent : theme.colors.text}
            />
          </Pressable>

          <Pressable
            onPress={() => setThumbRating.mutate(myRating === -1 ? 0 : -1)}
            disabled={thumbsBusy}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: myRating === -1 ? theme.colors.danger : theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
              opacity: thumbsBusy ? 0.5 : 1,
            }}
            accessible={true}
            accessibilityLabel={myRating === -1 ? t("media.rating.removeDislike") : t("media.rating.dislike")}
            accessibilityRole="button"
          >
            <Ionicons
              name={myRating === -1 ? "thumbs-down" : "thumbs-down-outline"}
              size={20}
              color={myRating === -1 ? theme.colors.onAccent : theme.colors.text}
            />
          </Pressable>
            </>
          )}

          {/* Deliberately NOT offered for a series/season: the server's
              MarkAsWatched writes a single progress row on the id it is given and
              nothing cascades it to the episodes (verified: no MediaMarkedWatched
              consumer descends the tree), so a "watched" series would leave every
              episode unwatched - a misleading button. Web shows it anyway; this is
              the one place we keep the mobile behaviour over web parity. */}
          {!demo && !isSeries && !isSeason && (
            <Pressable
              onPress={() => {
                if (!profileId) return;
                if (watched) {
                  markUnwatched.mutate({ mediaItemId: id!, params: { profileId } });
                } else {
                  markWatched.mutate({ mediaItemId: id!, params: { profileId } });
                }
              }}
              disabled={markWatched.isPending || markUnwatched.isPending}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
                opacity: (markWatched.isPending || markUnwatched.isPending) ? 0.5 : 1,
              }}
              accessible={true}
              accessibilityLabel={watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}
              accessibilityRole="button"
            >
              {(markWatched.isPending || markUnwatched.isPending) ? (
                <ActivityIndicator size={18} color={theme.colors.text} />
              ) : (
                <Ionicons
                  name={watched ? "checkmark-circle" : "checkmark-circle-outline"}
                  size={20}
                  color={watched ? theme.colors.accent : theme.colors.text}
                />
              )}
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}
              </Text>
            </Pressable>
          )}

          {demo ? null : (
          <Pressable
            onPress={() => setPlaylistSheetOpen(true)}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
            }}
            accessible={true}
            accessibilityLabel={t("media.detail.addToPlaylist")}
            accessibilityRole="button"
          >
            <Ionicons name="add" size={20} color={theme.colors.text} />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {t("media.detail.addToPlaylist")}
            </Text>
          </Pressable>
          )}

          {/* Sends this title's loadAndPlay command to one of the user's other
              connected Evey clients - the mobile counterpart to the web
              player's Cast/RemotePlaybackMenu "Play on" section. */}
          {/* Locked on the demo: every device there belongs to the shared account,
              so "Play on" would reach another visitor's screen. */}
          {demo ? null : (
          <Pressable
            onPress={() => setPlayOnSheetOpen(true)}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
            }}
            accessible={true}
            accessibilityLabel={t("player.playOn.button")}
            accessibilityRole="button"
          >
            <Ionicons name="tv-outline" size={20} color={theme.colors.text} />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {t("player.playOn.title")}
            </Text>
          </Pressable>
          )}
          {/* Grouping a title into a saga is thought of here, on the title's own
              page, not from a grid card - but only the library's bulk action bar
              offered it. Admin-gated like the RPC (AddCollectionItem is RoleAdmin). */}
          {!demo && isAdmin && id && (
            <Pressable
              onPress={() => setCollectionSheetOpen(true)}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
              }}
              accessible={true}
              accessibilityLabel={t("media.contextMenu.addToCollection")}
              accessibilityRole="button"
              testID="add-to-collection-button"
            >
              <Ionicons name="albums-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("media.contextMenu.addToCollection")}
              </Text>
            </Pressable>
          )}
          {!isSeries && !isSeason && hasMediaFile(media) && (
            <Pressable
              onPress={async () => {
                if (!id || !media?.title) return;
                if (downloadProgress !== null) return;
                const existing = await getDownloadPath(id);
                if (existing) { confirmDeleteDownload(id, media.title); return; }
                if (!(await confirmDownloadOnMobileData(t))) return;
                // A 4K remux can be tens of gigabytes: when the server offers
                // a lighter copy (and the operator turned the choice on), ask
                // before spending the space rather than after failing on it.
                const choice = await chooseDownloadQuality(t, id);
                if (choice.quality === "cancel") return;
                const sliced = choice.quality === "reduced" && !!choice.plan;
                setDownloadProgress(NaN);
                toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
                try {
                  if (sliced && choice.plan) {
                    // The sliced path builds its own file, so it reports
                    // through the same progress stream rather than returning
                    // one task to await.
                    const unsubscribe = subscribeToProgress(id, (p, b) => {
                      if (!screenMountedRef.current) return;
                      setDownloadProgress(p);
                      if (b) setDownloadBytes(b);
                    });
                    try {
                      await startReducedDownload(id, media.title, choice.plan.durationMs, choice.plan.slices, {
                        authContext: buildDownloadAuthContext(activeProfile, {
                          officialRating: media.officialRating,
                          certificationsByCountry: media.certificationsByCountry,
                        }),
                      });
                    } finally {
                      unsubscribe();
                    }
                    toast.success(t("media.detail.downloadComplete"));
                    return;
                  }
                  await startDownload(id, media.title,
                    (p, b) => { if (!screenMountedRef.current) return; setDownloadProgress(p); if (b) setDownloadBytes(b); },
                    {
                      ...buildDownloadAuthContext(activeProfile, {
                        officialRating: media.officialRating,
                        certificationsByCountry: media.certificationsByCountry,
                      }),
                      // Without the hint the extension falls back to ".mp4"
                      // even for Matroska bytes (the episode site always
                      // passed it; this one forgot) - which misnames the file
                      // for VLC and used to make the audio pass skip it.
                      formatHint: media.containerFormat,
                      audioCodecs: audioCodecsOf(media),
                      // The server stamps seriesTitle on episode payloads, so
                      // a lone episode downloaded from its own page still
                      // names its show on the offline list.
                      display: {
                        seriesTitle: media.seriesTitle,
                        seasonNumber: parentItem?.indexNumber,
                        episodeNumber: media.indexNumber,
                      },
                    },
                  );
                  toast.success(t("media.detail.downloadComplete"));
                } catch (err) {
                  // Surface the real cause instead of a blanket "failed": a
                  // disk-space rejection right after the "started" toast reads
                  // as a contradictory success+error to the user (reported on
                  // a 2.8 GB file). The sliced path fails its own way - a slice
                  // the server never produced, a join the device refused - and
                  // only reducedDownloadMessage names those.
                  toast.error(t(sliced ? reducedDownloadMessage(err) : downloadErrorMessageKey(err)));
                } finally {
                  setDownloadProgress(null);
                  setDownloadBytes(null);
                }
              }}
              disabled={downloadProgress !== null}
              style={{
                position: "relative",
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
                overflow: "hidden",
                opacity: downloadProgress !== null ? 0.85 : 1,
              }}
              accessible={true}
              accessibilityLabel={t("common.download") || "Download"}
              accessibilityRole="button"
            >
              {downloadProgress !== null && Number.isFinite(downloadProgress) && (
                <View style={{
                  position: "absolute",
                  left: 0,
                  top: 0,
                  bottom: 0,
                  width: `${Math.max(2, Math.min(100, downloadProgress * 100))}%`,
                  backgroundColor: theme.colors.accent,
                  opacity: 0.25,
                }} />
              )}
              <Ionicons
                name={downloadProgress !== null ? "cloud-download" : "download-outline"}
                size={20}
                color={theme.colors.text}
              />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {downloadProgress === null || !id
                  ? t("common.download") || "Download"
                  // Every "what is it doing right now" answer - server-side
                  // audio prep, 720p production, part N of M, assembling -
                  // comes from one place, shared with the Downloads screen.
                  : downloadStageLabel(t, id, downloadProgress, downloadBytes, 0, { formatSize: formatBytes })}
              </Text>
            </Pressable>
          )}
          {/* Share link, metadata editor and segment editor: locked on the demo,
              and none of the three has a read-only view on the phone. */}
          {/* A season or series link serves a ZIP. Same condition on the web. */}
          {!demo && isAdmin && id && (isSeries || isSeason || hasMediaFile(media)) && (
            <Pressable
              onPress={() => createShareLink.mutate({ mediaItemId: id, data: { durationHours: 48 } })}
              disabled={createShareLink.isPending}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
                opacity: createShareLink.isPending ? 0.6 : 1,
              }}
              accessible={true}
              accessibilityLabel={t("media.library.shareLink")}
              accessibilityRole="button"
            >
              <Ionicons name="share-social-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("media.library.shareLink")}
              </Text>
            </Pressable>
          )}
          {!demo && isAdmin && id && (
            <Pressable
              onPress={() => setMetadataSheetOpen(true)}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
              }}
              accessible={true}
              accessibilityLabel={t("media.detail.editMetadata")}
              accessibilityRole="button"
            >
              <Ionicons name="create-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("media.detail.editMetadata")}
              </Text>
            </Pressable>
          )}
          {!demo && isAdmin && id && (
            <Pressable
              onPress={() => router.push(`/admin/media/${id}` as never)}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
              }}
              accessible={true}
              accessibilityLabel={t("admin.segments.title", "Edit segments")}
              accessibilityRole="button"
            >
              <Ionicons name="settings-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("admin.segments.title", "Edit segments")}
              </Text>
            </Pressable>
          )}
        </View>
        </View>{/* end left column */}

        {/* Right column (landscape) / continues below (portrait) */}
        <View style={isLandscape ? { flex: 7 } : {}}>
        {/* Tagline */}
        {media.tagline ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, fontStyle: "italic", marginTop: theme.spacing.md }}>
            {media.tagline}
          </Text>
        ) : null}

        {/* Overview */}
        {media.overview ? (
          <Text style={{ color: theme.colors.text, fontSize: 14, lineHeight: 20, marginTop: theme.spacing.sm }}>
            {media.overview}
          </Text>
        ) : null}

        {/* Directors */}
        {directors.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.sm }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.director")}</Text>
            {directors.map((d) => d.name).filter(Boolean).join(", ")}
          </Text>
        ) : null}

        {/* Writers */}
        {writers.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 4 }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.writer")}</Text>
            {writers.map((w) => w.name).filter(Boolean).join(", ")}
          </Text>
        ) : null}

        {/* Studios */}
        {media.studios && media.studios.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 4 }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.studio")}</Text>
            {media.studios.join(", ")}
          </Text>
        ) : null}

        {/* Trailers */}
        {sortedTrailers.length > 0 ? (
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: theme.spacing.md }}>
            {sortedTrailers.map((tr, i) => (
              <TrailerButton key={`${tr.key}-${i}`} trailer={tr} />
            ))}
          </View>
        ) : null}

        {/* File info sits here, not at the bottom of the page: web puts it
            immediately after the hero (which carries the synopsis and the
            crew), before the seasons. Buried under cast and similar titles it
            was effectively unreachable on a phone. */}
        <FileInfoSection item={media} />

        {/* Season picker for series */}
        {isSeries && seasons.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              {t("media.detail.seasons")}
            </Text>
            <ScrollView horizontal showsHorizontalScrollIndicator={false}>
              {seasons.map((season) => {
                const active = season.id === selectedSeason;
                const label = seasonLabel(season, t);
                return (
                  <Pressable
                    key={season.id}
                    onPress={() => setSelectedSeason(season.id ?? null)}
                    style={{
                      paddingHorizontal: theme.spacing.md,
                      paddingVertical: theme.spacing.sm,
                      marginRight: theme.spacing.xs,
                      borderRadius: theme.radii.md,
                      backgroundColor: active ? theme.colors.accent : theme.colors.surface,
                    }}
                    accessible={true}
                    accessibilityLabel={label}
                    accessibilityRole="tab"
                  >
                    <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                      {label}
                    </Text>
                  </Pressable>
                );
              })}
            </ScrollView>
          </View>
        ) : null}

        {/* Episodes list */}
        {(isSeries || isSeason) && episodes.length > 0 ? (
          <View style={{ marginTop: theme.spacing.md }}>
            <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: theme.spacing.sm }}>
              <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600" }}>
                {t("media.detail.episodes")}
              </Text>
              <View style={{ flexDirection: "row", alignItems: "center", gap: theme.spacing.md }}>
              {episodes.some((ep) => hasMediaFile(ep)) ? (
                <Pressable
                  onPress={() => { void handleDownloadAllEpisodes(); }}
                  disabled={bulkDownload !== null}
                  style={{ flexDirection: "row", alignItems: "center", gap: 4, opacity: bulkDownload !== null ? 0.6 : 1 }}
                  accessible={true}
                  accessibilityLabel={t("media.detail.downloadAllEpisodes", "Download all")}
                  accessibilityRole="button"
                >
                  {bulkDownload !== null ? (
                    <ActivityIndicator size={14} color={theme.colors.accent} />
                  ) : (
                    <Ionicons name="download-outline" size={16} color={theme.colors.accent} />
                  )}
                  <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
                    {bulkDownload !== null
                      ? `${bulkDownload.done}/${bulkDownload.total}`
                      : t("media.detail.downloadAllEpisodes", "Download all")}
                  </Text>
                </Pressable>
              ) : null}
              {isSeries && seasons.length > 1 && bulkDownload === null ? (
                <Pressable
                  onPress={() => { void handleDownloadSeries(); }}
                  style={{ flexDirection: "row", alignItems: "center", gap: 4 }}
                  accessible={true}
                  accessibilityLabel={t("media.detail.downloadSeries", "Download series")}
                  accessibilityRole="button"
                >
                  <Ionicons name="albums-outline" size={16} color={theme.colors.accent} />
                  <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
                    {t("media.detail.downloadSeries", "Download series")}
                  </Text>
                </Pressable>
              ) : null}
              </View>
            </View>
            {episodes.slice(0, visibleEpisodeCount).map((ep) => (
              <EpisodeRow
                key={ep.id}
                item={ep}
                serverUrl={serverUrl}
                theme={theme}
                authHeaders={authHeaders}
                downloadProgress={ep.id ? episodeDownloads[ep.id] : undefined}
                progress={ep.id ? episodeProgressMap.get(ep.id) : undefined}
                onDownload={handleEpisodeDownload}
                onPlay={handleEpisodePlay}
              />
            ))}
            {episodes.length > visibleEpisodeCount ? (
              <Pressable
                onPress={() => setVisibleEpisodeCount((c) => c + EPISODE_PAGE_SIZE)}
                style={{ paddingVertical: theme.spacing.md, alignItems: "center" }}
                accessible={true}
                accessibilityLabel={t("media.detail.showMoreEpisodes", "Show more")}
                accessibilityRole="button"
              >
                <Text style={{ color: theme.colors.accent, fontSize: 14, fontWeight: "600" }}>
                  {t("media.detail.showMoreEpisodes", "Show more")}
                </Text>
              </Pressable>
            ) : null}
          </View>
        ) : null}

        {/* Cast */}
        {actors.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              {t("media.detail.cast")}
            </Text>
            <FlatList
              horizontal
              data={actors}
              keyExtractor={personKeyExtractor}
              renderItem={renderCastItem}
              showsHorizontalScrollIndicator={false}
            />
          </View>
        ) : null}

        {/* Crew */}
        {crew.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              {t("media.detail.crew")}
            </Text>
            <FlatList
              horizontal
              data={crew}
              keyExtractor={personKeyExtractor}
              renderItem={renderCastItem}
              showsHorizontalScrollIndicator={false}
            />
          </View>
        ) : null}

        {/* Sagas this title belongs to - just before Similar, as on the web. */}
        {id ? <SagaSection mediaItemId={id} /> : null}

        {/* Similar content */}
        {id && !isSeason ? <SimilarSection item={media} /> : null}

        {/* Plugin modules declared for the media page - same placement as
            web's MediaDetail (after similar titles). */}
        {id ? <PluginSlot name="media.detail.sidebar" context={{ mediaId: id }} /> : null}

        {/* Bottom spacing */}
        <View style={{ height: theme.spacing.xl * 2 }} />
        </View>{/* end right column */}
      </View>

      {id ? (
        <AddToPlaylistSheet
          visible={playlistSheetOpen}
          mediaItemId={id}
          onClose={() => setPlaylistSheetOpen(false)}
        />
      ) : null}

      {id ? (
        <PlayOnSheet
          visible={playOnSheetOpen}
          mediaId={id}
          onClose={() => setPlayOnSheetOpen(false)}
        />
      ) : null}

      {/* Defense in depth: re-check isAdmin at the mount point, not just at the
          toggle button, so a stale collectionSheetOpen=true can never render the
          sheet for a non-admin (e.g. after an admin demotes their own account
          mid-session). */}
      {isAdmin && id ? (
        <BulkAddToCollectionSheet
          visible={collectionSheetOpen}
          mediaItemIds={[id]}
          onClose={() => setCollectionSheetOpen(false)}
        />
      ) : null}

      {gateProfileId ? (
        <ParentalPINModal
          visible={showPinModal}
          onClose={() => { setShowPinModal(false); setPendingPlayTarget(null); }}
          onVerified={handlePinVerified}
          profileId={gateProfileId}
        />
      ) : null}

      {/* Defense in depth: re-check isAdmin at the mount point, not just at the
          toggle button, so a stale metadataSheetOpen=true can never render the
          editor for a non-admin (e.g. after an admin demotes their own account
          mid-session). */}
      {isAdmin && media ? (
        <MetadataEditSheet
          visible={metadataSheetOpen}
          onClose={() => setMetadataSheetOpen(false)}
          item={media}
          onSaved={() => void refetch()}
        />
      ) : null}
    </ScrollView>
  );
}
