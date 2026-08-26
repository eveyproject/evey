import { useCallback, useEffect, useRef, useState } from "react";
import { View, Text, ActivityIndicator, StyleSheet, useTVEventHandler, AccessibilityInfo } from "react-native";
import { Image } from "expo-image";
import { router, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { usePhotoServiceListPhotos, hasMoreFromCursor } from "@evey/api";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { photoServeUrl, buildPhotoListParams, type TvPhoto } from "../../../src/lib/photoGrid";
import {
  SLIDESHOW_AUTOPLAY_INTERVAL_MS,
  prevSlideshowIndex,
  nextSlideshowIndex,
  clampSlideshowIndex,
  nextAutoplayIndexPaged,
} from "../../../src/lib/photoSlideshow";

// The listing is PAGED, one keyset page at a time. It used to ask for 1000 in a
// single call, which the server never served: any limit above maxPageSize (500)
// is replaced by the photo handler's own default of 100 (clampLimitOffset,
// grpcapi/helpers.go). So the slideshow silently held 100 photos of any album -
// measured on a real 228-photo album - and opening it on a photo the grid had
// already paged past showed a DIFFERENT picture, because the out-of-range start
// index was clamped into the 100 that happened to be loaded.
// Only metadata is paged here (id/title/blurhash, no image bytes); photo bytes
// are still fetched one at a time below, plus one preload.
const SLIDESHOW_PAGE_SIZE = 100;

// How close to the end of the loaded set the viewer may get before the next
// page is requested. Autoplay advances one photo every 6s, so a few photos of
// head start is a comfortable margin even on a slow box.
const SLIDESHOW_PREFETCH_MARGIN = 10;

export default function PhotoSlideshowScreen() {
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const { libraryId, album, startIndex } = useLocalSearchParams<{
    libraryId: string;
    album?: string;
    startIndex?: string;
  }>();

  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [photos, setPhotos] = useState<TvPhoto[]>([]);
  const [albumTotal, setAlbumTotal] = useState(0);
  const { data, isLoading, isError, isFetching, refetch } = usePhotoServiceListPhotos(
    buildPhotoListParams({ libraryId: libraryId ?? "", album, limit: SLIDESHOW_PAGE_SIZE, cursor }),
    { query: { enabled: !!libraryId } },
  );

  // Append each page as it lands. Guarded on the response identity so a
  // re-render that hands back the same object does not duplicate a page.
  const lastPageRef = useRef<unknown>(undefined);
  useEffect(() => {
    if (!data || data === lastPageRef.current) return;
    lastPageRef.current = data;
    const page = (data.photos ?? []) as TvPhoto[];
    setPhotos((prev) => (cursor === undefined ? page : [...prev, ...page]));
    // Only the first page carries an exact album total - a keyset page reports
    // 0 by design. The counter shows the album's size, not how much of it has
    // been fetched, so it does not creep upward as pages land.
    if (cursor === undefined) setAlbumTotal(data.total ?? 0);
  }, [data, cursor]);

  const total = photos.length;
  const hasMore = hasMoreFromCursor({ nextCursor: data?.nextCursor });

  const [index, setIndex] = useState(() => Math.max(Number(startIndex ?? 0) || 0, 0));
  const [paused, setPaused] = useState(false);

  // Pull the next page while the viewer is getting close to the end of what is
  // loaded - or while the requested start index is not loaded yet, which is the
  // case whenever the grid was paged past the first page before opening one.
  useEffect(() => {
    if (isFetching || !hasMore) return;
    if (index >= total - SLIDESHOW_PREFETCH_MARGIN) setCursor(data?.nextCursor);
  }, [isFetching, hasMore, index, total, data?.nextCursor]);

  // Clamp only once the whole album is in hand. Clamping earlier is what made
  // an out-of-range start index land on the wrong photo instead of waiting for
  // the page that holds the right one - and clamping against an EMPTY list is
  // worse still: it resolves every start index to 0, so the very first pass
  // (before the page has been appended to state) would silently discard the
  // photo the viewer picked in the grid.
  useEffect(() => {
    if (hasMore || total === 0) return;
    setIndex((i) => clampSlideshowIndex(i, total));
  }, [total, hasMore]);

  const exit = useCallback(() => {
    if (router.canGoBack()) router.back();
    else router.replace("/(auth)/photos");
  }, []);

  // Autoplay - cleared whenever paused or the album can't advance (0 or 1
  // photo). Re-created per (paused, total) change so a concurrent library
  // scan changing the album size is picked up on the next tick rather than
  // advancing against stale bounds forever.
  useEffect(() => {
    if (paused || total <= 1) return;
    const id = setInterval(() => {
      setIndex((i) => nextAutoplayIndexPaged(i, total, hasMore));
    }, SLIDESHOW_AUTOPLAY_INTERVAL_MS);
    return () => clearInterval(id);
  }, [paused, total, hasMore]);

  const current = photos[index];
  const upcoming = total > 0 ? photos[(index + 1) % total] : undefined;

  // Screen-reader label for the photo currently on screen - the counter text
  // already shown in the footer, reused rather than duplicated in a second key.
  const photoAnnouncement = t("media.photos.slideshowAnnounce", {
    current: index + 1,
    total: albumTotal || total,
  });

  // Announce on index change only (not on every render, e.g. the autoplay
  // interval re-subscribing): TalkBack has no other way to learn the photo
  // changed since this screen renders no focus-driven navigation.
  useEffect(() => {
    if (!current?.id) return;
    AccessibilityInfo.announceForAccessibility(photoAnnouncement);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- announce on index change, not every re-render of photoAnnouncement's closure
  }, [current?.id]);

  // Preload at most the next photo, full resolution - never the whole album.
  useEffect(() => {
    if (!serverUrl || !upcoming?.id || upcoming.id === current?.id) return;
    Image.prefetch(photoServeUrl(serverUrl, upcoming.id), { headers: authHeaders }).catch(() => {
      // Best-effort: a failed preload just means the next photo loads
      // on-demand instead of instantly, not a broken slideshow.
    });
  }, [serverUrl, upcoming?.id, current?.id, authHeaders]);

  const goPrev = useCallback(() => setIndex((i) => prevSlideshowIndex(i, total)), [total]);
  const goNext = useCallback(() => setIndex((i) => nextSlideshowIndex(i, total)), [total]);
  const togglePause = useCallback(() => setPaused((p) => !p), []);

  // Global remote handler, same convention as the video player: this screen
  // has no on-screen focusable controls for its normal (photo showing)
  // state, so D-pad left/right/select/back are interpreted directly rather
  // than routed through native TV focus. Back/menu always exits - the
  // remote can never get stranded here even though nothing is focusable.
  // Stable subscription, fresh logic - see the same pattern in the TV player.
  // useTVEventHandler re-subscribes whenever the handler's identity changes, so
  // an inline arrow tears the native listener down on every render; here that is
  // every autoplay tick, and a D-pad press landing in the gap is dropped.
  const remoteHandlerRef = useRef<(evt: { eventType: string } | undefined) => void>(() => {});
  remoteHandlerRef.current = (evt: { eventType: string } | undefined) => {
    if (!evt) return;
    switch (evt.eventType) {
      case "left":
        goPrev();
        break;
      case "right":
        goNext();
        break;
      case "select":
      case "playPause":
        togglePause();
        break;
      case "menu":
      case "back":
        exit();
        break;
      default:
        break;
    }
  };
  useTVEventHandler(useCallback((evt: { eventType: string } | undefined) => remoteHandlerRef.current?.(evt), []));

  // Also covers "the photo we were asked to open lives in a page that has not
  // landed yet": showing the spinner beats showing a black frame, and beats the
  // old behaviour of clamping onto whichever photo happened to be loaded.
  if (isLoading || (total > 0 && !current && hasMore)) {
    return (
      <View style={[styles.container, styles.center]}>
        <ActivityIndicator color="#fff" size="large" />
      </View>
    );
  }

  // Only an EMPTY slideshow is an error screen. A later page failing must not
  // tear down a slideshow the viewer is already watching.
  if (total === 0) {
    return (
      <View style={[styles.container, styles.center]}>
        <Text style={styles.message}>{isError ? t("common.loadError") : t("media.photos.noPhotos")}</Text>
        {isError && (
          <FocusableCard onPress={() => refetch()} style={styles.actionButton} accessibilityLabel={t("common.retry")}>
            <Text style={styles.actionText}>{t("common.retry")}</Text>
          </FocusableCard>
        )}
        <FocusableCard onPress={exit} hasTVPreferredFocus style={styles.actionButton} accessibilityLabel={t("common.aria.close")}>
          <Text style={styles.actionText}>{t("common.aria.close")}</Text>
        </FocusableCard>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      {current?.id && (
        // focusable + accessible: this screen has no other focus-reachable
        // node (D-pad is read directly via useTVEventHandler above, not
        // native focus), so a screen reader would otherwise have nothing to
        // land on. Purely a focus/announcement target - pause/next/prev stay
        // wired through the remote handler above, unchanged.
        <View
          style={StyleSheet.absoluteFill}
          focusable
          accessible
          accessibilityRole="image"
          accessibilityLabel={photoAnnouncement}
        >
          <Image
            key={current.id}
            source={{ uri: photoServeUrl(serverUrl, current.id), headers: authHeaders }}
            style={StyleSheet.absoluteFill}
            contentFit="contain"
            placeholder={current.blurhash ? { blurhash: current.blurhash } : undefined}
            placeholderContentFit="contain"
            transition={200}
          />
        </View>
      )}

      <View style={styles.footer} pointerEvents="none">
        {(current?.title || current?.album) && (
          <Text style={styles.title} numberOfLines={1}>
            {current?.title || current?.album}
          </Text>
        )}
        <Text style={styles.counter}>{t("media.photos.positionOf", { current: index + 1, total: albumTotal || total })}</Text>
        {paused && <Text style={styles.paused}>{t("media.photos.slideshowPaused")}</Text>}
      </View>
    </View>
  );
}

// A fullscreen slideshow keeps fixed white-on-black chrome rather than theme
// tokens, same convention as the video player's overlay - it lives on a
// permanently black background regardless of the active theme.
const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#000" },
  center: { justifyContent: "center", alignItems: "center" },
  message: { color: "#fff", fontSize: 22, marginBottom: 16, textAlign: "center", paddingHorizontal: 60 },
  actionButton: { paddingHorizontal: 24, paddingVertical: 14, marginTop: 12 },
  actionText: { color: "#fff", fontSize: 18, fontWeight: "600" },
  footer: {
    position: "absolute",
    bottom: 32,
    left: 0,
    right: 0,
    alignItems: "center",
  },
  title: { color: "#fff", fontSize: 20, fontWeight: "500", marginBottom: 4 },
  counter: { color: "rgba(255,255,255,0.7)", fontSize: 16 },
  paused: { color: "rgba(255,255,255,0.9)", fontSize: 16, fontWeight: "600", marginTop: 6 },
});
