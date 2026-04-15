import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  useLibraryServiceListLibraries,
  usePhotoServiceListPhotos,
  usePhotoServiceListPhotoAlbums,
  photoServiceDeletePhoto,
} from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { EmptyState } from "@/components/ui/EmptyState";
import { Images, ChevronLeft, ChevronRight, X, Trash2, RotateCcw, RotateCw } from "lucide-react";

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

function usePhotos(libraryID: string | undefined, album?: string, limit = 5000, offset = 0) {
  return usePhotoServiceListPhotos(
    { libraryId: libraryID, limit, offset, album },
    {
      query: {
        enabled: !!libraryID,
        select: (d) => ({
          photos: (d.photos ?? []).map<Photo>((p) => ({
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
          })),
          total: d.total ?? 0,
        }),
        staleTime: 30_000,
      },
    },
  );
}

// ---- Lightbox ----

interface LightboxProps {
  photos: Photo[];
  index: number;
  onClose: () => void;
  onPrev: () => void;
  onNext: () => void;
  onDelete?: (photo: Photo) => void;
}

function Lightbox({ photos, index, onClose, onPrev, onNext, onDelete }: LightboxProps) {
  const photo = photos[index];
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const [rotation, setRotation] = useState(0);

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
    <div className="fixed inset-0 z-overlay flex items-center justify-center bg-black/90" onClick={onClose}>
      <div className="absolute top-4 right-4 flex items-center gap-3 z-raised">
        <button
          onClick={(e) => { e.stopPropagation(); setRotation((r) => (r + 270) % 360); }}
          className="transition-colors-fast text-[var(--overlay-text)]"
          aria-label="Rotate left"
          title="Rotate left (Shift+R)"
        >
          <RotateCcw size={22} />
        </button>
        <button
          onClick={(e) => { e.stopPropagation(); setRotation((r) => (r + 90) % 360); }}
          className="transition-colors-fast text-[var(--overlay-text)]"
          aria-label="Rotate right"
          title="Rotate right (R)"
        >
          <RotateCw size={22} />
        </button>
        {onDelete && (
          <button
            onClick={(e) => { e.stopPropagation(); onDelete(photo); }}
            className="transition-colors-fast text-[var(--overlay-text-muted)]"
            aria-label="Delete photo"
          >
            <Trash2 size={22}  aria-hidden="true" />
          </button>
        )}
        <button
          onClick={(e) => { e.stopPropagation(); onClose(); }}
          className="transition-colors-fast text-[var(--overlay-text)]"
          aria-label="Close"
        >
          <X size={28}  aria-hidden="true" />
        </button>
      </div>

      {index > 0 && (
        <button
          onClick={(e) => { e.stopPropagation(); onPrev(); }}
          className="absolute left-2 sm:left-4 top-1/2 -translate-y-1/2 transition-colors-fast z-raised p-3 sm:p-2 rounded-full bg-black/40 hover:bg-black/60 text-[var(--overlay-text)]"
          aria-label="Previous"
        >
          <ChevronLeft size={28}  aria-hidden="true" />
        </button>
      )}

      {index < photos.length - 1 && (
        <button
          onClick={(e) => { e.stopPropagation(); onNext(); }}
          className="absolute right-2 sm:right-4 top-1/2 -translate-y-1/2 transition-colors-fast z-raised p-3 sm:p-2 rounded-full bg-black/40 hover:bg-black/60 text-[var(--overlay-text)]"
          aria-label="Next"
        >
          <ChevronRight size={28}  aria-hidden="true" />
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
          {photo.Album && <span className="ml-2 text-[var(--text-muted)]">— {photo.Album}</span>}
          <span className="ml-2 text-[var(--text-muted)]">{index + 1} / {photos.length}</span>
        </div>
      </div>
    </div>
  );
}

// ---- Photo grid ----

interface PhotoGridProps {
  photos: Photo[];
  onPhotoClick: (index: number) => void;
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

function PhotoGrid({ photos, onPhotoClick }: PhotoGridProps) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const thumbParam = `?${accessToken ? `token=${encodeURIComponent(accessToken)}&` : ""}w=400&fmt=webp`;
  const parentRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(800);
  const [scrollEl, setScrollEl] = useState<HTMLElement | null>(null);

  useEffect(() => {
    const el = parentRef.current;
    if (!el) return;
    setScrollEl(findScrollParent(el));
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width;
      if (w) setContainerWidth(w);
    });
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

  if (photos.length === 0) return <EmptyState title={t("media.photos.noPhotos")} description={t("media.photos.noPhotosDesc")} />;

  return (
    <div ref={parentRef}>
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map((vRow) => (
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
                <button
                  key={photo.ID}
                  onClick={() => onPhotoClick(globalIdx)}
                  className="group relative aspect-square overflow-hidden rounded-[var(--radius-md)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] cursor-pointer bg-[var(--surface-raised)]"
                  aria-label={photo.Title}
                >
                  <img
                    src={`/api/v1/photos/${photo.ID}/serve${thumbParam}`}
                    alt={photo.Title}
                    loading="lazy"
                    className="w-full h-full object-cover transition-transform duration-200 group-hover:scale-105"
                  />
                  <div className="absolute inset-0 bg-black/0 group-hover:bg-black/20 transition-colors-fast duration-200" />
                </button>
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
}

function AlbumGrid({ albums, onAlbumClick }: AlbumGridProps) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const thumbParam = `?${accessToken ? `token=${encodeURIComponent(accessToken)}&` : ""}w=400&fmt=webp`;
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
          <p className="text-xs text-[var(--text-muted)]">{album.PhotoCount} photo{album.PhotoCount !== 1 ? "s" : ""}</p>
        </button>
      ))}
    </div>
  );
}

// ---- Main page ----

type View = "albums" | "photos";

function PhotosPage() {
  const { t } = useTranslation();
  const { data: libraries, isLoading: libLoading } = usePhotoLibraries();
  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [view, setView] = useState<View>("albums");
  const [selectedAlbum, setSelectedAlbum] = useState<string | undefined>(undefined);
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const qc = useQueryClient();

  const firstLib = libraries?.[0];
  const activeLibraryID = selectedLibraryID ?? firstLib?.id;

  const { data: albumsData, isLoading: albumsLoading } = useAlbums(activeLibraryID);
  const { data: photosData, isLoading: photosLoading } = usePhotos(activeLibraryID, selectedAlbum);

  const displayedPhotos = photosData?.photos ?? [];

  const openLightbox = useCallback((index: number) => setLightboxIndex(index), []);
  const closeLightbox = useCallback(() => setLightboxIndex(null), []);
  const prevPhoto = useCallback(() => setLightboxIndex((i) => (i !== null && i > 0 ? i - 1 : i)), []);
  const nextPhoto = useCallback(() => setLightboxIndex((i) => (i !== null && i < displayedPhotos.length - 1 ? i + 1 : i)), [displayedPhotos.length]);

  const deletePhotoMutation = useMutation({
    mutationFn: (id: string) => photoServiceDeletePhoto(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["photos"] });
      setLightboxIndex(null);
      toast.success(t("media.photos.photoDeleted"));
    },
    onError: (err: Error) => toast.error(err.message || t("media.photos.failedDeletePhoto")),
  });

  function handleDeletePhoto(photo: Photo) {
    if (!window.confirm(t("media.photos.deletePhotoConfirm", { title: photo.Title }))) return;
    deletePhotoMutation.mutate(photo.ID);
  }

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
              onPhotoClick={openLightbox}
            />
          )}
          {(photosData?.total ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)] text-right">
              {photosData?.total} photo{(photosData?.total ?? 0) !== 1 ? "s" : ""}
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
          onDelete={isAdmin ? handleDeletePhoto : undefined}
        />
      )}
    </div>
  );
}
