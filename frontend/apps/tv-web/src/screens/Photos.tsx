import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  getBaseURL,
  hasMoreFromCursor,
  useLibraryServiceListLibraries,
  usePhotoServiceListPhotoAlbums,
  usePhotoServiceListPhotos,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import {
  PHOTO_GRID_THUMB_WIDTH,
  SLIDESHOW_AUTOPLAY_INTERVAL_MS,
  buildPhotoListParams,
  nextAutoplayIndexPaged,
  nextSlideshowIndex,
  photoServeUrl,
  prevSlideshowIndex,
  type TvPhoto,
} from "@evey/client";
import { FocusRoot, useTvKeys } from "../tv/focus";
import { Pressable, Spinner } from "../components/ui";
import { Icon } from "../components/icons";
import { ScreenState } from "../components/ScreenState";
import { StackHeader } from "../components/StackHeader";
import { goBack, navigate } from "../app/router";

const PAGE_SIZE = 60;

// The photo endpoints are loaded by the browser itself, so the token rides
// the query string the way the artwork does.
function photoSrc(photoId: string, width?: number): string {
  const token = useAuthStore.getState().accessToken;
  const base = photoServeUrl(getBaseURL(), photoId, width ? { width } : undefined);
  return token ? `${base}${base.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : base;
}

/** A photo library, as the Android television shows it: albums as chips, photos as square tiles. */
export function PhotosScreen({ libraryId, albumId }: { libraryId?: string; albumId?: string }) {
  const { t } = useTranslation();
  const libs = useLibraryServiceListLibraries({
    query: { select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_PHOTOS" && l.id), staleTime: 60_000 },
  });
  const photoLibs = libs.data ?? [];
  const [chosen, setChosen] = useState<string | undefined>(libraryId);
  const activeId = chosen ?? photoLibs[0]?.id;
  const [album, setAlbum] = useState<string | undefined>(albumId);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [photos, setPhotos] = useState<TvPhoto[]>([]);

  const albumsQ = usePhotoServiceListPhotoAlbums({ libraryId: activeId ?? "" }, { query: { enabled: !!activeId } });
  const { data, isLoading, isFetching, isError, refetch } = usePhotoServiceListPhotos(
    buildPhotoListParams({ libraryId: activeId ?? "", album, limit: PAGE_SIZE, cursor }),
    { query: { enabled: !!activeId, gcTime: 30_000 } },
  );

  // Reset before accumulating: a cached first page must not be wiped by a
  // reset that runs second.
  useEffect(() => { setCursor(undefined); setPhotos([]); }, [activeId, album]);
  useEffect(() => {
    if (!data) return;
    const fresh = (data.photos ?? []) as TvPhoto[];
    setPhotos((prev) => (cursor ? [...prev, ...fresh] : fresh));
  }, [data, cursor]);

  const hasMore = hasMoreFromCursor({ nextCursor: data?.nextCursor });
  const albums = albumsQ.data?.albums ?? [];

  if (libs.isLoading) return <ScreenState loading />;
  if (photoLibs.length === 0) {
    return (
      <FocusRoot autoFocus className="center-fill pad-top">
        <p className="muted big">{t("media.photos.noPhotoLibraries")}</p>
        <Pressable className="tv-card empty-action" autoFocus onPress={() => navigate({ name: "libraries" })}>{t("media.library.libraries")}</Pressable>
      </FocusRoot>
    );
  }

  return (
    <div className="deep-page">
      <StackHeader title={t("media.photos.title")} />
      {isError ? (
        <FocusRoot autoFocus className="center-fill">
          <p className="muted big">{t("common.loadError")}</p>
          <Pressable className="tv-card link-btn" autoFocus onPress={() => void refetch()}>{t("common.retry")}</Pressable>
        </FocusRoot>
      ) : (
        <FocusRoot autoFocus className="library-body">
          {photoLibs.length > 1 ? (
            <FocusRoot className="photo-libs">
              {photoLibs.map((l) => (
                <Pressable key={l.id} className={`tv-card music-lib ${l.id === activeId ? "active" : ""}`} onPress={() => setChosen(l.id)}>{l.name}</Pressable>
              ))}
            </FocusRoot>
          ) : null}
          {albumsQ.isLoading ? (
            <div className="photo-albums-wait"><Spinner /></div>
          ) : albums.length > 0 ? (
            <FocusRoot className="photo-albums">
              <Pressable className={`tv-card album-chip ${!album ? "active" : ""}`} label={t("media.photos.allPhotos")} onPress={() => setAlbum(undefined)}>
                <span className="album-chip-art"><Icon name="images-outline" size={28} /></span>
                <span className="album-chip-name">{t("media.photos.allPhotos")}</span>
              </Pressable>
              {albums.map((a) => (
                <Pressable key={a.name} className={`tv-card album-chip ${album === a.name ? "active" : ""}`} label={a.name} onPress={() => setAlbum(album === a.name ? undefined : a.name)}>
                  <span className="album-chip-art">
                    {a.coverPhotoId ? <img src={photoSrc(a.coverPhotoId, PHOTO_GRID_THUMB_WIDTH)} alt="" loading="lazy" /> : <Icon name="images-outline" size={28} />}
                  </span>
                  <span className="album-chip-name">{a.name}</span>
                  <span className="album-chip-count">{a.photoCount ?? 0}</span>
                </Pressable>
              ))}
            </FocusRoot>
          ) : null}
          {isLoading && !cursor ? (
            <div className="center-fill pad-top"><Spinner /></div>
          ) : photos.length === 0 ? (
            <FocusRoot className="center-fill pad-top">
              <p className="muted big">{t("media.photos.noPhotos")}</p>
              <Pressable className="tv-card empty-action" onPress={() => navigate({ name: "libraries" })}>{t("media.library.libraries")}</Pressable>
            </FocusRoot>
          ) : (
            <FocusRoot grid className="photo-grid-wrap">
              <div className="grid photo-tiles">
                {photos.map((p, i) => (
                  <Pressable
                    key={p.id ?? i}
                    focusKey={`photos:${p.id}`}
                    className="tv-card photo-tile"
                    autoFocus={i === 0}
                    label={p.title || p.album || t("media.photos.openPhoto", { n: i + 1, defaultValue: `Photo ${i + 1}` })}
                    onFocus={() => {
                      const next = data?.nextCursor;
                      if (hasMore && !isFetching && next && next !== cursor && i >= photos.length - 12) setCursor(next);
                    }}
                    onPress={() => navigate({ name: "slideshow", libraryId: activeId, albumId: album, index: i })}
                  >
                    {p.id ? <img src={photoSrc(p.id, PHOTO_GRID_THUMB_WIDTH)} alt="" loading="lazy" /> : null}
                  </Pressable>
                ))}
              </div>
              {isFetching && cursor ? <Spinner /> : null}
            </FocusRoot>
          )}
        </FocusRoot>
      )}
    </div>
  );
}

/** The full-bleed slideshow: left and right walk it, OK pauses, back leaves. */
export function SlideshowScreen({ libraryId, albumId, index = 0 }: { libraryId?: string; albumId?: string; index?: number }) {
  const { t } = useTranslation();
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [photos, setPhotos] = useState<TvPhoto[]>([]);
  const [at, setAt] = useState(index);
  const [paused, setPaused] = useState(false);

  const { data, isLoading, isError, refetch } = usePhotoServiceListPhotos(
    buildPhotoListParams({ libraryId: libraryId ?? "", album: albumId, limit: PAGE_SIZE, cursor }),
    { query: { enabled: !!libraryId } },
  );
  useEffect(() => {
    if (!data) return;
    const fresh = (data.photos ?? []) as TvPhoto[];
    setPhotos((prev) => (cursor ? [...prev, ...fresh] : fresh));
  }, [data, cursor]);

  const total = photos.length;
  // Near the end of what is loaded, ask for the next page so the walk never
  // stops at a page boundary.
  useEffect(() => {
    const next = data?.nextCursor;
    if (next && next !== cursor && at >= total - 5) setCursor(next);
  }, [at, total, data, cursor]);

  // Autoplay loops; while a page is still coming it holds on the last loaded
  // photo instead of sending the viewer back to the first.
  const hasMore = !!data?.nextCursor;
  useEffect(() => {
    if (paused || total === 0) return;
    const id = setInterval(() => setAt((i) => nextAutoplayIndexPaged(i, total, hasMore)), SLIDESHOW_AUTOPLAY_INTERVAL_MS);
    return () => clearInterval(id);
  }, [paused, total, hasMore]);

  useTvKeys((key, e) => {
    if (key === "back") { e.preventDefault(); goBack(); }
    else if (key === "playPause" || key === "play" || key === "pause") setPaused((p) => !p);
  });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight" || e.keyCode === 39) { e.preventDefault(); setAt((i) => nextSlideshowIndex(i, total)); }
      else if (e.key === "ArrowLeft" || e.keyCode === 37) { e.preventDefault(); setAt((i) => prevSlideshowIndex(i, total)); }
      else if ((e.key === "Enter" || e.keyCode === 13) && total > 0) { e.preventDefault(); setPaused((p) => !p); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [total]);

  const current = photos[Math.min(at, Math.max(0, total - 1))];
  const upcoming = total > 0 ? photos[(at + 1) % total] : undefined;
  const src = useMemo(() => (current?.id ? photoSrc(current.id) : undefined), [current]);
  // The next photo is fetched ahead, so the change lands without a blank.
  useEffect(() => {
    if (!upcoming?.id || upcoming.id === current?.id) return;
    const img = new Image();
    img.src = photoSrc(upcoming.id);
  }, [upcoming?.id, current?.id]);

  if (isLoading) return <div className="slideshow"><Spinner /></div>;
  if (total === 0) {
    return (
      <FocusRoot autoFocus className="slideshow slideshow-empty">
        <p className="slideshow-message">{isError ? t("common.loadError") : t("media.photos.noPhotos")}</p>
        {isError ? <Pressable className="tv-card slideshow-action" onPress={() => void refetch()}>{t("common.retry")}</Pressable> : null}
        <Pressable className="tv-card slideshow-action" autoFocus onPress={goBack}>{t("common.aria.close")}</Pressable>
      </FocusRoot>
    );
  }

  return (
    <div className="slideshow" role="img" aria-label={t("media.photos.slideshowAnnounce", { current: at + 1, total })}>
      {src ? <img key={current?.id} src={src} alt="" /> : null}
      <div className="slideshow-footer">
        {current?.title || current?.album ? <span className="slideshow-title">{current?.title || current?.album}</span> : null}
        <span className="slideshow-counter">{t("media.photos.positionOf", { current: at + 1, total })}</span>
        {paused ? <span className="slideshow-paused">{t("media.photos.slideshowPaused")}</span> : null}
      </div>
    </div>
  );
}
