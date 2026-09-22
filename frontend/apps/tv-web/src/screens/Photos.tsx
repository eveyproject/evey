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
import { Button, Card, Spinner } from "../components/ui";
import { goBack, navigate } from "../app/router";

const PAGE_SIZE = 60;

// The photo endpoints are loaded by the browser itself, so the token rides
// the query string the way the artwork does.
function photoSrc(photoId: string, width?: number): string {
  const token = useAuthStore.getState().accessToken;
  const base = photoServeUrl(getBaseURL(), photoId, width ? { width } : undefined);
  return token ? `${base}${base.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : base;
}

/** A photo library: its albums, then its photos, paged as the remote walks. */
export function PhotosScreen({ libraryId, albumId }: { libraryId?: string; albumId?: string }) {
  const { t } = useTranslation();
  const { data: libs } = useLibraryServiceListLibraries();
  const photoLibs = (libs?.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_PHOTOS" && l.id);
  const [chosen, setChosen] = useState<string | undefined>(libraryId);
  const activeId = chosen ?? photoLibs[0]?.id;
  const [album, setAlbum] = useState<string | undefined>(albumId);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [photos, setPhotos] = useState<TvPhoto[]>([]);

  const { data: albumsData } = usePhotoServiceListPhotoAlbums({ libraryId: activeId ?? "" }, { query: { enabled: !!activeId } });
  const { data, isLoading, isFetching } = usePhotoServiceListPhotos(
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
  const albums = albumsData?.albums ?? [];

  if (photoLibs.length === 0) return <div className="screen screen-center"><p className="muted">{t("media.photos.noPhotoLibraries")}</p></div>;

  return (
    <div className="screen">
      <FocusRoot autoFocus className="header">
        <h1 className="title">{t("media.photos.title")}</h1>
        <div className="actions" style={{ margin: 0 }}>
          {photoLibs.map((l) => (
            <Button key={l.id} variant={l.id === activeId ? "primary" : "default"} onPress={() => setChosen(l.id)}>{l.name}</Button>
          ))}
        </div>
      </FocusRoot>
      {albums.length > 0 ? (
        <FocusRoot className="chips">
          <Button variant={!album ? "primary" : "default"} onPress={() => setAlbum(undefined)}>{t("media.photos.allPhotos")}</Button>
          {albums.map((a) => (
            <Button key={a.name} variant={album === a.name ? "primary" : "default"} onPress={() => setAlbum(a.name)}>
              {a.name}{a.photoCount ? ` (${a.photoCount})` : ""}
            </Button>
          ))}
        </FocusRoot>
      ) : null}
      {isLoading ? <Spinner /> : null}
      <FocusRoot>
        <div className="grid photo-grid">
          {photos.map((p, i) => (
            <Card
              key={p.id ?? i}
              title={p.title ?? ""}
              imageUrl={p.id ? photoSrc(p.id, PHOTO_GRID_THUMB_WIDTH) : undefined}
              wide
              onFocus={() => {
                const next = data?.nextCursor;
                if (hasMore && !isFetching && next && next !== cursor && i >= photos.length - 12) setCursor(next);
              }}
              onPress={() => navigate({ name: "slideshow", libraryId: activeId, albumId: album, index: i })}
            />
          ))}
        </div>
      </FocusRoot>
    </div>
  );
}

/** The full-bleed slideshow: left and right walk it, OK holds or resumes. */
export function SlideshowScreen({ libraryId, albumId, index = 0 }: { libraryId?: string; albumId?: string; index?: number }) {
  const { t } = useTranslation();
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [photos, setPhotos] = useState<TvPhoto[]>([]);
  const [at, setAt] = useState(index);
  const [playing, setPlaying] = useState(true);

  const { data } = usePhotoServiceListPhotos(
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

  // Autoplay loops where a key press holds: a slideshow left running at a
  // gathering keeps cycling rather than freezing on the last frame. While a
  // page is still coming it holds on the last loaded photo instead of
  // sending the viewer back to the first.
  const hasMore = !!data?.nextCursor;
  useEffect(() => {
    if (!playing || total === 0) return;
    const id = setInterval(() => setAt((i) => nextAutoplayIndexPaged(i, total, hasMore)), SLIDESHOW_AUTOPLAY_INTERVAL_MS);
    return () => clearInterval(id);
  }, [playing, total, hasMore]);

  useTvKeys((key, e) => {
    if (key === "back") { e.preventDefault(); goBack(); }
    else if (key === "playPause" || key === "play" || key === "pause") setPlaying((p) => !p);
  });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight") { e.preventDefault(); setAt((i) => nextSlideshowIndex(i, total)); }
      else if (e.key === "ArrowLeft") { e.preventDefault(); setAt((i) => prevSlideshowIndex(i, total)); }
      else if (e.key === "Enter") { e.preventDefault(); setPlaying((p) => !p); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [total]);

  const current = photos[Math.min(at, Math.max(0, total - 1))];
  const src = useMemo(() => (current?.id ? photoSrc(current.id) : undefined), [current]);

  return (
    <div className="slideshow" role="img" aria-label={current?.title ?? t("media.photos.title")}>
      {src ? <img src={src} alt={current?.title ?? ""} /> : <Spinner />}
      <div className="slideshow-meta">
        {current?.title ? <span>{current.title}</span> : null}
        <span className="muted">{total > 0 ? `${at + 1} / ${total}` : ""}{playing ? "" : ` · ${t("common.pause")}`}</span>
      </div>
    </div>
  );
}
