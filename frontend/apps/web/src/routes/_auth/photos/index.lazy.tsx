import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useInfiniteQuery } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  photoServiceListPhotos,
  usePhotoServiceListPhotoAlbums,
  usePhotoServiceGetPhoto,
  hasMoreFromCursor,
} from "@evey/api";
import { useImageToken } from "@/lib/imageToken";
import { useTranslation } from "react-i18next";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { useFocusTrap } from "@/hooks/useFocusTrap";
import { Images, ChevronLeft, ChevronRight, X, RotateCcw, RotateCw } from "lucide-react";
import { Blurhash } from "react-blurhash";
import { ICON } from "@/constants/iconSizes";
import { formatBytes } from "@evey/api";

export const Route = createLazyFileRoute("/_auth/photos/")({
  component: PhotosPage,
});

// ---- Types ----

interface Photo {
  ID: string;
  LibraryID: string;
  Title: string;
  FilePath: string;
  FileSize: number;
  MimeType: string;
  Width: number;
  Height: number;
  TakenAt?: string;
  Album: string;
  CreatedAt: string;
  // Optional: empty for files ingested before migration 000096 or whose
  // codec the encoder couldn't decode (e.g. HEIC, RAW).
  Blurhash?: string;
}

interface Album {
  Name: string;
  LibraryID: string;
  PhotoCount: number;
  CoverPhotoID: string;
}

// ---- API hooks ----

interface LibraryItem {
  id: string;
  name: string;
  type: string;
}

function usePhotoLibraries() {
  return useLibraryServiceListLibraries({
    query: {
      select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_PHOTOS").map<LibraryItem>((l) => ({ id: l.id ?? "", name: l.name ?? "", type: l.type ?? "" })),
      staleTime: 60_000,
    },
  });
}

function useAlbums(libraryID: string | undefined) {
  return usePhotoServiceListPhotoAlbums(
    { libraryId: libraryID },
    {
      query: {
        enabled: !!libraryID,
        select: (d) => (d.albums ?? []).map<Album>((a) => ({
          Name: a.name ?? "",
          LibraryID: a.libraryId ?? "",
          PhotoCount: a.photoCount ?? 0,
          CoverPhotoID: a.coverPhotoId ?? "",
        })),
        staleTime: 30_000,
      },
    },
  );
}

// Photos are keyset-paginated, one page at a time. The page used to ask for
// limit=5000 in a single call, which never worked as intended: the server
// replaces any limit above maxPageSize (500) with its own default of 100
// (clampLimitOffset, grpcapi/helpers.go), so a library of 7000 photos rendered
// exactly 100 tiles under a header proudly announcing the real total. Page 1
// takes the offset path (the only one that computes an exact total); every
// page after it carries the previous response's cursor.
const PHOTO_PAGE_SIZE = 100;

function usePhotos(libraryID: string | undefined, album?: string) {
  const query = useInfiniteQuery({
    queryKey: ["photos-infinite", libraryID ?? "", album ?? ""],
    enabled: !!libraryID,
    initialPageParam: "" as string,
    queryFn: ({ pageParam, signal }) =>
      photoServiceListPhotos(
        {
          libraryId: libraryID,
          limit: PHOTO_PAGE_SIZE,
          ...(pageParam ? { cursor: pageParam } : { offset: 0 }),
          ...(album ? { album } : {}),
        },
        signal,
      ),
    getNextPageParam: (last) => (hasMoreFromCursor(last) ? last.nextCursor : undefined),
    staleTime: 30_000,
  });

  const photos = useMemo(
    () =>
      (query.data?.pages ?? []).flatMap((page) =>
        (page.photos ?? []).map<Photo>((p) => ({
          ID: p.id ?? "",
          LibraryID: p.libraryId ?? "",
          Title: p.title ?? "",
          FilePath: p.filePath ?? "",
          FileSize: Number(p.fileSize ?? 0),
          MimeType: p.mimeType ?? "",
          Width: p.width ?? 0,
          Height: p.height ?? 0,
          TakenAt: p.takenAt,
          Album: p.album ?? "",
          CreatedAt: p.createdAt ?? "",
          Blurhash: p.blurhash ?? "",
        })),
      ),
    [query.data],
  );

  // Only the first page carries an exact total - a keyset page reports 0 by
  // design, that COUNT(*) OVER() being the cost the cursor path exists to
  // avoid. Reading it from any other page would zero the counter mid-scroll.
  const total = query.data?.pages[0]?.total ?? 0;

  return { ...query, photos, total };
}

// GetPhoto returns cameraModel, which ListPhotos doesn't project - the rest
// of the EXIF strip (width/height/fileSize/takenAt) is already on the Photo
// the caller has, so this only fetches the one field missing from the list.
function usePhotoCameraModel(photoId: string | undefined) {
  return usePhotoServiceGetPhoto(photoId ?? "", {
    query: {
      enabled: !!photoId,
      select: (d) => d.cameraModel ?? "",
      staleTime: 60_000,
    },
  });
}

function formatTakenAt(takenAt: string): string {
  return new Date(takenAt).toLocaleDateString(undefined, {
    year: "numeric",
    month: "long",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// ---- Lightbox ----

interface LightboxProps {
  photos: Photo[];
  index: number;
  onClose: () => void;
  onPrev: () => void;
  onNext: () => void;
}

function Lightbox({ photos, index, onClose, onPrev, onNext }: LightboxProps) {
  const { t } = useTranslation();
  const photo = photos[index];
  const accessToken = useImageToken();
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const [rotation, setRotation] = useState(0);
  const dialogRef = useRef<HTMLDivElement>(null);
  const { data: cameraModel } = usePhotoCameraModel(photo?.ID);

  useFocusTrap(dialogRef, Boolean(photo));

  useEffect(() => { setRotation(0); }, [index]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowLeft") onPrev();
      else if (e.key === "ArrowRight") onNext();
      else if (e.key === "r") setRotation((r) => (r + 90) % 360);
      else if (e.key === "R") setRotation((r) => (r + 270) % 360);
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose, onPrev, onNext]);

  if (!photo) return null;

  return (
    <div
      ref={dialogRef}
      tabIndex={-1}
      className="fixed inset-0 z-overlay flex items-center justify-center bg-black/90 outline-none"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={t("media.photos.lightbox", "Photo viewer")}
    >
      <div className="absolute top-4 right-4 flex items-center gap-3 z-raised">
        <button
          onClick={(e) => { e.stopPropagation(); setRotation((r) => (r + 270) % 360); }}
          className="transition-colors-fast text-[var(--overlay-text)]"
          aria-label={t("common.rotateLeft")}
          title={`${t("common.rotateLeft")} (Shift+R)`}
        >
          <RotateCcw size={ICON.xl} />
        </button>
        <button
          onClick={(e) => { e.stopPropagation(); setRotation((r) => (r + 90) % 360); }}
          className="transition-colors-fast text-[var(--overlay-text)]"
          aria-label={t("common.rotateRight")}
          title={`${t("common.rotateRight")} (R)`}
        >
          <RotateCw size={ICON.xl} />
        </button>
        <button
          onClick={(e) => { e.stopPropagation(); onClose(); }}
          className="transition-colors-fast text-[var(--overlay-text)]"
          aria-label={t("common.aria.close")}
        >
          <X size={ICON["2xl"]}  aria-hidden="true" />
        </button>
      </div>

      {index > 0 && (
        <button
          onClick={(e) => { e.stopPropagation(); onPrev(); }}
          className="absolute left-2 sm:left-4 top-1/2 -translate-y-1/2 transition-colors-fast z-raised p-3 sm:p-2 rounded-full bg-black/40 hover:bg-black/60 text-[var(--overlay-text)]"
          aria-label={t("common.previous")}
        >
          <ChevronLeft size={ICON["2xl"]}  aria-hidden="true" />
        </button>
      )}

      {index < photos.length - 1 && (
        <button
          onClick={(e) => { e.stopPropagation(); onNext(); }}
          className="absolute right-2 sm:right-4 top-1/2 -translate-y-1/2 transition-colors-fast z-raised p-3 sm:p-2 rounded-full bg-black/40 hover:bg-black/60 text-[var(--overlay-text)]"
          aria-label={t("common.next")}
        >
          <ChevronRight size={ICON["2xl"]}  aria-hidden="true" />
        </button>
      )}

      <div className="max-w-[90vw] max-h-[90vh] flex flex-col items-center gap-3" onClick={(e) => e.stopPropagation()}>
        <img
          src={`/api/v1/photos/${photo.ID}/serve${tokenParam}`}
          alt={photo.Title}
          className="max-w-full max-h-[80vh] object-contain rounded transition-transform"
          style={{ transform: `rotate(${rotation}deg)` }}
        />
        <div className="text-white text-sm text-center">
          <span className="font-medium">{photo.Title}</span>
          {photo.Album && <span className="ml-2 text-[var(--text-muted)]">- {photo.Album}</span>}
          <span className="ml-2 text-[var(--text-muted)]">{index + 1} / {photos.length}</span>
        </div>
        <div className="flex flex-col items-center gap-0.5 text-xs text-[var(--text-muted)]">
          {photo.Width > 0 && photo.Height > 0 && (
            <span>{photo.Width} x {photo.Height}</span>
          )}
          {photo.FileSize > 0 && <span>{formatBytes(photo.FileSize)}</span>}
          {photo.TakenAt && <span>{formatTakenAt(photo.TakenAt)}</span>}
          {cameraModel && <span>{cameraModel}</span>}
        </div>
      </div>
    </div>
  );
}

// ---- Photo grid ----

// PhotoCell renders one tile with a blurhash-driven placeholder visible
// until the thumbnail finishes loading. Each tile owns its loaded state so
// the cells in the same row don't flash collectively when one image
// resolves before its neighbours.
function PhotoCell({ photo, thumbParam, onClick }: { photo: Photo; thumbParam: string; onClick: () => void }) {
  const [loaded, setLoaded] = useState(false);
  return (
    <button
      onClick={onClick}
      className="group relative aspect-square overflow-hidden rounded-[var(--radius-md)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] cursor-pointer bg-[var(--surface-raised)]"
      aria-label={photo.Title}
    >
      {photo.Blurhash && !loaded ? (
        <div className="absolute inset-0">
          <Blurhash
            hash={photo.Blurhash}
            width="100%"
            height="100%"
            resolutionX={32}
            resolutionY={32}
            punch={1}
          />
        </div>
      ) : null}
      <img
        src={`/api/v1/photos/${photo.ID}/serve${thumbParam}`}
        alt={photo.Title}
        loading="lazy"
        onLoad={() => setLoaded(true)}
        className="w-full h-full object-cover transition-transform duration-200 group-hover:scale-105 relative"
        style={{ opacity: loaded ? 1 : 0 }}
      />
      <div className="absolute inset-0 bg-black/0 group-hover:bg-black/20 transition-colors-fast duration-200" />
    </button>
  );
}

interface PhotoGridProps {
  photos: Photo[];
  onPhotoClick: (index: number) => void;
  isError?: boolean;
  error?: unknown;
  onRetry?: () => void;
  // Must stay referentially stable and do its own gating: the effect below
  // fires on every scroll that reaches the last row, so a caller that guards
  // outside (by swapping the prop for undefined mid-fetch) would leave the
  // grid stuck on the page it was showing when the fetch started.
  onEndReached?: () => void;
}

const PHOTO_COL_BREAKPOINTS = [
  { minWidth: 1280, cols: 6 },
  { minWidth: 1024, cols: 5 },
  { minWidth: 768, cols: 4 },
  { minWidth: 640, cols: 3 },
  { minWidth: 0, cols: 2 },
];
const PHOTO_GAP = 8;

function getPhotoCols(width: number): number {
  for (const bp of PHOTO_COL_BREAKPOINTS) {
    if (width >= bp.minWidth) return bp.cols;
  }
  return 2;
}

function findScrollParent(el: HTMLElement | null): HTMLElement {
  let node: HTMLElement | null = el?.parentElement ?? null;
  while (node) {
    const style = getComputedStyle(node);
    if (/(auto|scroll|overlay)/.test(style.overflowY)) return node;
    node = node.parentElement;
  }
  return (document.scrollingElement as HTMLElement) ?? document.documentElement;
}

function PhotoGrid({ photos, onPhotoClick, isError, error, onRetry, onEndReached }: PhotoGridProps) {
  const { t } = useTranslation();
  const accessToken = useImageToken();
  const thumbParam = `?${accessToken ? `token=${encodeURIComponent(accessToken)}&` : ""}w=400&fmt=webp`;
  const parentRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(800);
  const [scrollEl, setScrollEl] = useState<HTMLElement | null>(null);

  useEffect(() => {
    const el = parentRef.current;
    if (!el) return;
    setScrollEl(findScrollParent(el));
    const onResize: ResizeObserverCallback = (entries) => {
      const w = entries[0]?.contentRect.width;
      if (w) setContainerWidth(w);
    };
    const ro = new ResizeObserver(onResize);
    ro.observe(el);
    setContainerWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const cols = getPhotoCols(containerWidth);
  const cellSize = (containerWidth - PHOTO_GAP * (cols - 1)) / cols;

  const rows = useMemo(() => {
    const result: Photo[][] = [];
    for (let i = 0; i < photos.length; i += cols) {
      result.push(photos.slice(i, i + cols));
    }
    return result;
  }, [photos, cols]);

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => cellSize + PHOTO_GAP,
    overscan: 3,
    scrollMargin: parentRef.current && scrollEl ? parentRef.current.getBoundingClientRect().top - scrollEl.getBoundingClientRect().top + scrollEl.scrollTop : 0,
  });

  const virtualRows = virtualizer.getVirtualItems();
  const lastRenderedRow = virtualRows.length > 0 ? virtualRows[virtualRows.length - 1].index : -1;

  // Ask for the next page as soon as the last built row is rendered. The
  // virtualizer's overscan means that happens a few rows before the user sees
  // the bottom, so the grid keeps filling ahead of the scroll.
  useEffect(() => {
    if (rows.length === 0 || lastRenderedRow < rows.length - 1) return;
    onEndReached?.();
  }, [lastRenderedRow, rows.length, onEndReached]);

  // A failed photo fetch must not collapse into "no photos" - that reads as an
  // empty album/library when the server actually errored.
  if (isError) return <FetchErrorState error={error} onRetry={onRetry ?? (() => {})} />;
  if (photos.length === 0) return <EmptyState title={t("media.photos.noPhotos")} description={t("media.photos.noPhotosDesc")} />;

  return (
    <div ref={parentRef}>
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualRows.map((vRow) => (
          <div
            key={vRow.index}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              right: 0,
              transform: `translateY(${vRow.start - virtualizer.options.scrollMargin}px)`,
              height: cellSize + PHOTO_GAP,
              display: "grid",
              gridTemplateColumns: `repeat(${cols}, 1fr)`,
              gap: PHOTO_GAP,
            }}
          >
            {rows[vRow.index].map((photo, colIdx) => {
              const globalIdx = vRow.index * cols + colIdx;
              return (
                <PhotoCell
                  key={photo.ID}
                  photo={photo}
                  thumbParam={thumbParam}
                  onClick={() => onPhotoClick(globalIdx)}
                />
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
}

// ---- Album grid ----

interface AlbumGridProps {
  albums: Album[];
  onAlbumClick: (album: Album) => void;
  isError?: boolean;
  error?: unknown;
  onRetry?: () => void;
}

function AlbumGrid({ albums, onAlbumClick, isError, error, onRetry }: AlbumGridProps) {
  const { t } = useTranslation();
  const accessToken = useImageToken();
  const thumbParam = `?${accessToken ? `token=${encodeURIComponent(accessToken)}&` : ""}w=400&fmt=webp`;
  // A failed album fetch must not collapse into "no albums" - that reads as an
  // empty library when the server actually errored.
  if (isError) return <FetchErrorState error={error} onRetry={onRetry ?? (() => {})} />;
  if (albums.length === 0) return <EmptyState title={t("media.photos.noAlbums")} description={t("media.photos.noAlbumsDesc")} />;
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4">
      {albums.map((album) => (
        <button
          key={album.Name}
          onClick={() => onAlbumClick(album)}
          className="group text-left focus:outline-none focus:ring-2 focus:ring-[var(--accent)] rounded-[var(--radius-lg)] cursor-pointer"
        >
          <div
            className="relative overflow-hidden rounded-[var(--radius-lg)] aspect-square mb-2 bg-[var(--surface-raised)]"
          >
            {album.CoverPhotoID ? (
              <img
                src={`/api/v1/photos/${album.CoverPhotoID}/serve${thumbParam}`}
                alt={album.Name}
                loading="lazy"
                className="w-full h-full object-cover transition-transform duration-200 group-hover:scale-105"
              />
            ) : (
              <div className="w-full h-full flex items-center justify-center">
                <Images size={40} className="text-[var(--text-muted)]" />
              </div>
            )}
            <div className="absolute inset-0 bg-black/0 group-hover:bg-black/20 transition-colors-fast duration-200" />
          </div>
          <p className="text-sm font-medium text-[var(--text)] truncate" title={album.Name}>{album.Name.split("/").pop()}</p>
          <p className="text-xs text-[var(--text-muted)]">{t("media.photos.photoCount", { count: album.PhotoCount })}</p>
        </button>
      ))}
    </div>
  );
}

// ---- Main page ----

type View = "albums" | "photos";

// How close to the end of the loaded set the lightbox may walk before the next
// page is requested. A few photos of head start is enough for the fetch to
// land before the user gets there, at one keypress per photo.
const LIGHTBOX_PREFETCH_MARGIN = 5;

function PhotosPage() {
  const { t } = useTranslation();
  const { data: libraries, isLoading: libLoading, isError: libError, error: libErrorObj, refetch: refetchLibraries } = usePhotoLibraries();
  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [view, setView] = useState<View>("albums");
  const [selectedAlbum, setSelectedAlbum] = useState<string | undefined>(undefined);
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);

  const firstLib = libraries?.[0];
  const activeLibraryID = selectedLibraryID ?? firstLib?.id;

  const { data: albumsData, isLoading: albumsLoading, isError: albumsError, error: albumsErrorObj, refetch: refetchAlbums } = useAlbums(activeLibraryID);
  const {
    photos: displayedPhotos,
    total: photoTotal,
    isLoading: photosLoading,
    isError: photosError,
    error: photosErrorObj,
    refetch: refetchPhotos,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = usePhotos(activeLibraryID, selectedAlbum);

  // Stable identity, gated inside: both the grid's end-of-scroll and the
  // lightbox walking past the last loaded photo go through here.
  const loadMorePhotos = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const openLightbox = useCallback((index: number) => setLightboxIndex(index), []);
  const closeLightbox = useCallback(() => setLightboxIndex(null), []);
  const prevPhoto = useCallback(() => setLightboxIndex((i) => (i !== null && i > 0 ? i - 1 : i)), []);
  const nextPhoto = useCallback(() => setLightboxIndex((i) => (i !== null && i < displayedPhotos.length - 1 ? i + 1 : i)), [displayedPhotos.length]);

  // Close the lightbox whenever the displayed photo set changes (switching album
  // or library). Otherwise the lightbox renders unconditionally on lightboxIndex,
  // so a stale index would index into the NEW set - showing the wrong photo, or
  // auto-opening the lightbox on a photo the user never clicked.
  useEffect(() => {
    setLightboxIndex(null);
  }, [selectedAlbum, activeLibraryID]);

  // Stepping through the lightbox must not stop at the last loaded page. Its
  // Next button is bounded by displayedPhotos.length, so without this the
  // slideshow would dead-end at the end of the current page even though the
  // library has more - the exact wall the grid used to hit at 100 photos.
  useEffect(() => {
    if (lightboxIndex === null) return;
    if (lightboxIndex >= displayedPhotos.length - LIGHTBOX_PREFETCH_MARGIN) loadMorePhotos();
  }, [lightboxIndex, displayedPhotos.length, loadMorePhotos]);

  if (libLoading) {
    return (
      <div className="flex-1 p-6">
        <div className="animate-pulse h-8 w-48 rounded mb-6 bg-[var(--surface-raised)]" />
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <div key={i} className="animate-pulse aspect-square rounded-[var(--radius-lg)] bg-[var(--surface-raised)]" />
          ))}
        </div>
      </div>
    );
  }

  if (libError) {
    // Without this branch a failed fetch fell through to "no photo libraries",
    // telling the user their photos vanished instead of the server being
    // unreachable.
    return (
      <div className="flex-1 p-6">
        <FetchErrorState error={libErrorObj} onRetry={() => void refetchLibraries()} />
      </div>
    );
  }

  if (!libraries || libraries.length === 0) {
    return (
      <div className="flex-1 p-6">
        <EmptyState title={t("media.photos.noPhotoLibraries")} description={t("media.photos.noPhotoLibrariesDesc")} />
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col p-6 gap-4 overflow-y-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-4">
        <h1 className="text-2xl font-bold text-[var(--text)] flex items-center gap-2">
          <Images size={24} />
          {t("media.photos.title")}
        </h1>

        <div className="flex items-center gap-3 ml-auto flex-wrap">
          {/* Library selector */}
          {libraries.length > 1 && (
            <select
              value={activeLibraryID ?? ""}
              onChange={(e) => { setSelectedLibraryID(e.target.value); setSelectedAlbum(undefined); }}
              aria-label={t("media.library.selectLibrary")}
              className="text-sm px-3 py-1.5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] cursor-pointer"
            >
              {libraries.map((lib) => (
                <option key={lib.id} value={lib.id}>{lib.name}</option>
              ))}
            </select>
          )}

          {/* View toggle */}
          <div className="flex rounded-[var(--radius-sm)] border border-[var(--border)] overflow-hidden">
            <button
              onClick={() => { setView("albums"); setSelectedAlbum(undefined); }}
              className={`px-3 py-1.5 text-sm transition-colors-fast ${view === "albums" ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "bg-[var(--surface-raised)] text-[var(--text-secondary)] hover:bg-[var(--surface)]"}`}
            >
              {t("media.photos.albums")}
            </button>
            <button
              onClick={() => setView("photos")}
              // Stable hook for the e2e spec: matching the label would tie the
              // test to whichever language the profile happens to be in.
              data-testid="photos-view-all"
              className={`px-3 py-1.5 text-sm transition-colors-fast ${view === "photos" ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "bg-[var(--surface-raised)] text-[var(--text-secondary)] hover:bg-[var(--surface)]"}`}
            >
              {t("media.photos.allPhotos")}
            </button>
          </div>
        </div>
      </div>

      {/* Content */}
      {view === "albums" && !selectedAlbum && (
        <>
          {albumsLoading ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4">
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="animate-pulse aspect-square rounded-[var(--radius-lg)] bg-[var(--surface-raised)]" />
              ))}
            </div>
          ) : (
            <AlbumGrid
              albums={albumsData ?? []}
              isError={albumsError}
              error={albumsErrorObj}
              onRetry={() => void refetchAlbums()}
              onAlbumClick={(album) => { setSelectedAlbum(album.Name); setView("photos"); }}
            />
          )}
        </>
      )}

      {view === "photos" && (
        <>
          {selectedAlbum && (
            <div className="flex items-center gap-2">
              <button
                onClick={() => { setSelectedAlbum(undefined); setView("albums"); }}
                className="text-sm text-[var(--text-muted)] hover:text-[var(--text)] flex items-center gap-1 transition-colors-fast"
              >
                <ChevronLeft size={16} />
                {t("media.photos.albums")}
              </button>
              <span className="text-[var(--text-muted)]">/</span>
              <span className="text-sm font-medium text-[var(--text)]" title={selectedAlbum}>{selectedAlbum.split("/").pop()}</span>
            </div>
          )}
          {photosLoading ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-2">
              {Array.from({ length: 18 }).map((_, i) => (
                <div key={i} className="animate-pulse aspect-square rounded-[var(--radius-md)] bg-[var(--surface-raised)]" />
              ))}
            </div>
          ) : (
            <PhotoGrid
              photos={displayedPhotos}
              isError={photosError}
              error={photosErrorObj}
              onRetry={() => void refetchPhotos()}
              onPhotoClick={openLightbox}
              onEndReached={loadMorePhotos}
            />
          )}
          {photoTotal > 0 && (
            <p className="text-xs text-[var(--text-muted)] text-right">
              {t("media.photos.photoCount", { count: photoTotal })}
            </p>
          )}
        </>
      )}

      {/* Lightbox */}
      {lightboxIndex !== null && displayedPhotos.length > 0 && (
        <Lightbox
          photos={displayedPhotos}
          index={lightboxIndex}
          onClose={closeLightbox}
          onPrev={prevPhoto}
          onNext={nextPhoto}
        />
      )}
    </div>
  );
}
