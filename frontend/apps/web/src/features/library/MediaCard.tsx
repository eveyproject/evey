import { memo, useRef, useState, useEffect, useCallback, useMemo } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched } from "@evey/api";
import type { V1Artwork } from "@evey/api/src/generated/library.schemas";
import { toast, useAuthStore } from "@evey/store";
import { FavoriteButton } from "./FavoriteButton";
import { StarRatingBadge } from "./StarRating";
import { useIsFavorite, useToggleFavorite } from "@/hooks/useFavorite";
import { AddToPlaylistModal } from "../playlist/AddToPlaylistModal";
import { AddToCollectionModal } from "../collection/AddToCollectionModal";
import { typeLabel } from "@/utils/typeLabel";

export type { MediaCardItem };

interface MediaCardItem {
  id?: string;
  title?: string;
  year?: number;
  type?: string;
  overview?: string;
  communityRating?: number;
  genres?: string[];
  durationMs?: string;
  artwork?: V1Artwork[];
}

export interface MediaCardProps {
  item: MediaCardItem;
  href?: string;
  posterSrc?: string;
  showProgress?: boolean;
  progressPercent?: number;
  showYear?: boolean;
  showType?: boolean;
  isWatched?: boolean;
  resolution?: string;
  isHdr?: boolean;
  selected?: boolean;
  selectionMode?: boolean;
  onSelect?: (id: string, selected: boolean) => void;
}

export { typeLabel };

const formatResolution = (r: string) => (r === "2160p" ? "4K" : r);

const HoverPreview = memo(function HoverPreview({ item }: { item: MediaCardItem }) {
  const hasContent = item.overview || item.communityRating || (item.genres && item.genres.length > 0);
  if (!hasContent) return null;

  return (
    <div
      className="absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-56 rounded-lg p-3 shadow-xl z-50 pointer-events-none transition-opacity duration-200 hidden sm:block"
      style={{
        backgroundColor: "var(--surface)",
        border: "1px solid var(--border)",
      }}
    >
      <p className="text-sm font-semibold mb-1 truncate" style={{ color: "var(--text)" }}>
        {item.title}
      </p>

      <div className="flex items-center gap-2 mb-1.5 text-xs" style={{ color: "var(--text-muted)" }}>
        {item.year && <span>{item.year}</span>}
        {item.communityRating != null && item.communityRating > 0 && (
          <span className="flex items-center gap-0.5">
            <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor" style={{ color: "var(--accent)" }}>
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {item.communityRating.toFixed(1)}
          </span>
        )}
        {typeLabel(item.type) && <span>{typeLabel(item.type)}</span>}
      </div>

      {item.genres && item.genres.length > 0 && (
        <div className="flex flex-wrap gap-1 mb-1.5">
          {item.genres.slice(0, 3).map((g) => (
            <span
              key={g}
              className="text-[10px] px-1.5 py-0.5 rounded-full"
              style={{
                backgroundColor: "var(--surface-raised)",
                color: "var(--text-muted)",
              }}
            >
              {g}
            </span>
          ))}
        </div>
      )}

      {item.overview && (
        <p
          className="text-xs leading-relaxed line-clamp-3"
          style={{ color: "var(--text-secondary)" }}
        >
          {item.overview}
        </p>
      )}
    </div>
  );
});

const CardContextMenu = memo(function CardContextMenu({
  mediaId,
  onClose,
  position,
  onAddToPlaylist,
  onAddToCollection,
}: {
  mediaId: string;
  onClose: () => void;
  position: { x: number; y: number };
  onAddToPlaylist: () => void;
  onAddToCollection: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId);
  const { data: isFav } = useIsFavorite(mediaId);
  const toggleFav = useToggleFavorite(mediaId);
  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaId}`] });
        toast.success("Marked as watched");
        onClose();
      },
      onError: () => {
        toast.error("Failed to mark as watched");
        onClose();
      },
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaId}`] });
        toast.success("Marked as unwatched");
        onClose();
      },
      onError: () => {
        toast.error("Failed to mark as unwatched");
        onClose();
      },
    },
  });

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [onClose]);

  const items = [
    {
      label: "Edit metadata",
      onClick: () => {
        navigate({ to: "/media/$mediaId", params: { mediaId }, search: { edit: true } as never });
        onClose();
      },
    },
    {
      label: "Add to Playlist",
      onClick: () => {
        onClose();
        onAddToPlaylist();
      },
    },
    {
      label: "Add to Collection",
      onClick: () => {
        onClose();
        onAddToCollection();
      },
    },
    {
      label: isFav ? "Remove from Favorites" : "Add to Favorites",
      onClick: () => {
        toggleFav.mutate();
        onClose();
      },
    },
    {
      label: "Download",
      onClick: () => {
        window.open(`/api/v1/media/${mediaId}/download`, "_blank");
        onClose();
      },
    },
    {
      label: "Mark as watched",
      onClick: () => {
        if (!profileId) { toast.error("No profile selected"); onClose(); return; }
        markWatched.mutate({ mediaItemId: mediaId, params: { profileId } });
      },
    },
    {
      label: "Mark as unwatched",
      onClick: () => {
        if (!profileId) { toast.error("No profile selected"); onClose(); return; }
        markUnwatched.mutate({ mediaItemId: mediaId, params: { profileId } });
      },
    },
  ];

  return (
    <div
      ref={ref}
      className="fixed z-[100] min-w-[160px] py-1 rounded-[var(--radius-md)] shadow-xl"
      style={{
        left: position.x,
        top: position.y,
        backgroundColor: "var(--surface)",
        border: "1px solid var(--border)",
      }}
    >
      {items.map((it) => (
        <button
          key={it.label}
          onClick={it.onClick}
          className="w-full text-left px-3 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
          style={{ color: "var(--text)" }}
        >
          {it.label}
        </button>
      ))}
    </div>
  );
});

export const MediaCard = memo(function MediaCard({
  item,
  href,
  posterSrc,
  showProgress,
  progressPercent,
  showYear = true,
  showType,
  isWatched,
  resolution,
  isHdr,
  selected = false,
  selectionMode = false,
  onSelect,
}: MediaCardProps) {
  const [imgLoaded, setImgLoaded] = useState(false);
  const hoverTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [showPreview, setShowPreview] = useState(false);
  const accessToken = useAuthStore((s) => s.accessToken);

  const poster = useMemo(() => {
    const artworkPosterUrl = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url ?? null;
    return (
      posterSrc
      ?? artworkPosterUrl
      ?? (item.id && accessToken
        ? `/api/v1/media/${item.id}/artwork/poster?token=${encodeURIComponent(accessToken)}`
        : null)
    );
  }, [posterSrc, item.artwork, item.id, accessToken]);

  const initial = item.title ? item.title[0].toUpperCase() : "?";
  const pct =
    progressPercent !== undefined
      ? Math.min(100, Math.max(0, Math.round(progressPercent)))
      : undefined;

  const watched = isWatched || (pct !== undefined && pct >= 95);

  const linkTo = href ?? "/media/$mediaId";
  const linkParams = href ? undefined : { mediaId: item.id ?? "" };

  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number } | null>(null);
  const [playlistModalOpen, setPlaylistModalOpen] = useState(false);
  const [collectionModalOpen, setCollectionModalOpen] = useState(false);
  const menuBtnRef = useRef<HTMLButtonElement>(null);
  const hasPreviewData = !!(item.overview || item.communityRating || (item.genres && item.genres.length > 0));

  const handleMouseEnter = useCallback(() => {
    if (!hasPreviewData) return;
    hoverTimer.current = setTimeout(() => setShowPreview(true), 400);
  }, [hasPreviewData]);

  const handleMouseLeave = useCallback(() => {
    if (hoverTimer.current) clearTimeout(hoverTimer.current);
    setShowPreview(false);
  }, []);

  const handleFocus = useCallback(() => {
    if (!hasPreviewData) return;
    hoverTimer.current = setTimeout(() => setShowPreview(true), 400);
  }, [hasPreviewData]);

  const handleBlur = useCallback(() => {
    if (hoverTimer.current) clearTimeout(hoverTimer.current);
    setShowPreview(false);
  }, []);

  const handleContextMenu = useCallback((e: React.MouseEvent) => {
    if (!item.id) return;
    e.preventDefault();
    setShowPreview(false);
    setCtxMenu({ x: e.clientX, y: e.clientY });
  }, [item.id]);

  const handleSelectionClick = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    onSelect?.(item.id!, !selected);
  }, [onSelect, item.id, selected]);

  const hasProgressBar = showProgress && pct !== undefined;

  return (
    <>
    {item.id && playlistModalOpen && (
      <AddToPlaylistModal open={playlistModalOpen} onClose={() => setPlaylistModalOpen(false)} mediaItemId={item.id} />
    )}
    {item.id && collectionModalOpen && (
      <AddToCollectionModal open={collectionModalOpen} onClose={() => setCollectionModalOpen(false)} mediaItemIds={[item.id]} />
    )}
    {ctxMenu && item.id && (
      <CardContextMenu
        mediaId={item.id}
        position={ctxMenu}
        onClose={() => setCtxMenu(null)}
        onAddToPlaylist={() => setPlaylistModalOpen(true)}
        onAddToCollection={() => setCollectionModalOpen(true)}
      />
    )}
    <Link
      to={linkTo}
      params={linkParams as never}
      className={`group block flex-shrink-0 w-32 sm:w-40 relative${selected ? " ring-2 ring-[var(--accent)] rounded-[var(--radius-md)]" : ""}`}
      data-testid="media-card"
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
      onFocus={handleFocus}
      onBlur={handleBlur}
      onContextMenu={handleContextMenu}
      onClick={(onSelect && selectionMode) ? handleSelectionClick : undefined}
    >
      {showPreview && <HoverPreview item={item} />}

      <div
        className="relative w-full overflow-hidden transition-all duration-200 ease-out group-hover:scale-105 group-hover:shadow-[0_8px_24px_rgba(0,0,0,0.4)] group-focus-within:scale-105 group-focus-within:shadow-[0_8px_24px_rgba(0,0,0,0.4)]"
        style={{
          aspectRatio: "2/3",
          borderRadius: "var(--radius-md)",
          boxShadow: "0 2px 8px rgba(0,0,0,0.2)",
        }}
      >
        {poster ? (
          <img
            src={poster}
            alt={item.title ?? ""}
            loading="lazy"
            className="w-full h-full object-cover transition-opacity duration-300"
            style={{ opacity: imgLoaded ? 1 : 0 }}
            onLoad={() => setImgLoaded(true)}
            onError={(e) => {
              const t = e.currentTarget;
              t.style.display = "none";
              const fb = t.nextElementSibling as HTMLElement | null;
              if (fb) fb.style.display = "flex";
            }}
          />
        ) : null}
        <div
          className="absolute inset-0 items-center justify-center text-2xl font-bold"
          style={{
            backgroundColor: "var(--surface-raised)",
            color: "var(--text-muted)",
            display: poster ? "none" : "flex",
          }}
        >
          {initial}
        </div>

        {showYear && item.year !== undefined && (
          <span
            className="absolute top-2 right-2 text-xs font-medium px-1.5 py-0.5"
            style={{
              backgroundColor: "rgba(0,0,0,0.7)",
              color: "var(--overlay-text)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {item.year}
          </span>
        )}

        {watched && (
          <span
            className="absolute z-20 flex items-center justify-center"
            style={{
              top: "0.5rem",
              right: showYear && item.year !== undefined ? "3.5rem" : "0.5rem",
              width: "20px",
              height: "20px",
              borderRadius: "50%",
              backgroundColor: "var(--color-success)",
            }}
            data-testid="watched-badge"
          >
            <svg className="w-3 h-3 text-white" fill="none" stroke="currentColor" strokeWidth={3} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
            </svg>
          </span>
        )}

        {item.communityRating != null && item.communityRating > 0 && (
          <span
            className="absolute z-20 flex items-center gap-0.5 text-[10px] font-bold px-1 py-0.5 leading-none"
            style={{
              bottom: hasProgressBar ? "0.75rem" : "0.5rem",
              right: "0.5rem",
              backgroundColor: "rgba(0,0,0,0.72)",
              color: "#f5c518",
              borderRadius: "var(--radius-sm)",
            }}
          >
            <svg className="w-2.5 h-2.5" viewBox="0 0 20 20" fill="currentColor">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {item.communityRating.toFixed(1)}
          </span>
        )}

        {(resolution || isHdr) && (
          <div
            className="absolute z-20 flex items-center gap-1"
            style={{ bottom: hasProgressBar ? "0.75rem" : "0.5rem", left: "0.5rem" }}
          >
            {resolution && (
              <span
                className="text-[10px] font-bold px-1.5 py-0.5 leading-none"
                style={{
                  backgroundColor: "rgba(0,0,0,0.7)",
                  color: "var(--overlay-text)",
                  borderRadius: "var(--radius-sm)",
                }}
                data-testid="resolution-badge"
              >
                {formatResolution(resolution)}
              </span>
            )}
            {isHdr && (
              <span
                className="text-[10px] font-bold px-1.5 py-0.5 leading-none"
                style={{
                  backgroundColor: "#f59e0b",
                  color: "#000",
                  borderRadius: "var(--radius-sm)",
                }}
                data-testid="hdr-badge"
              >
                HDR
              </span>
            )}
          </div>
        )}

        {item.id && (onSelect || selectionMode) ? (
          <button
            aria-label={selected ? "Deselect" : "Select"}
            onClick={(e) => { e.preventDefault(); e.stopPropagation(); onSelect?.(item.id!, !selected); }}
            className={`absolute top-2 left-2 z-20 flex items-center justify-center w-5 h-5 rounded transition-colors${selectionMode && !selected ? " opacity-0 group-hover:opacity-100 group-focus-within:opacity-100" : ""}`}
            style={{
              backgroundColor: selected ? "var(--accent)" : "rgba(0,0,0,0.6)",
              border: `2px solid ${selected ? "var(--accent)" : "rgba(255,255,255,0.7)"}`,
            }}
          >
            {selected && (
              <svg className="w-3 h-3 text-white" fill="none" stroke="currentColor" strokeWidth={3} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
            )}
          </button>
        ) : item.id ? (
          <div className="absolute top-2 left-2 z-20 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-200">
            <FavoriteButton mediaId={item.id} size="sm" />
          </div>
        ) : null}

        {item.id && <StarRatingBadge mediaId={item.id} />}

        {item.id && (
          <button
            ref={menuBtnRef}
            aria-label="Options"
            aria-haspopup="true"
            aria-expanded={ctxMenu !== null}
            className="absolute top-2 right-2 z-20 opacity-0 group-hover:opacity-100 focus:opacity-100 p-1 rounded bg-black/60 text-white"
            style={{ lineHeight: 1 }}
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              const rect = menuBtnRef.current?.getBoundingClientRect();
              setCtxMenu(rect ? { x: rect.left, y: rect.bottom + 4 } : { x: e.clientX, y: e.clientY });
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                e.stopPropagation();
                const rect = menuBtnRef.current?.getBoundingClientRect();
                setCtxMenu(rect ? { x: rect.left, y: rect.bottom + 4 } : { x: 0, y: 0 });
              }
            }}
            data-testid="media-card-options-btn"
          >
            &#8942;
          </button>
        )}

        <div
          className="absolute inset-0 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-200"
          style={{ background: "linear-gradient(to top, rgba(0,0,0,0.8) 0%, rgba(0,0,0,0.3) 50%, rgba(0,0,0,0.1) 100%)" }}
        >
          <div
            role="img"
            aria-label="Play"
            className="w-12 h-12 rounded-full flex items-center justify-center backdrop-blur-sm transition-transform duration-200 group-hover:scale-110 group-focus-within:scale-110"
            style={{ backgroundColor: "rgba(255,255,255,0.2)", border: "2px solid rgba(255,255,255,0.6)" }}
          >
            <svg
              className="w-5 h-5 text-white ml-0.5"
              fill="currentColor"
              viewBox="0 0 20 20"
              aria-hidden="true"
            >
              <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
            </svg>
          </div>
          <p className="absolute bottom-2 left-2 right-2 text-xs font-medium text-white truncate text-center">
            {item.title}
          </p>
        </div>

        {hasProgressBar && (
          <div
            className="absolute bottom-0 left-0 right-0 h-1.5 z-10"
            style={{ backgroundColor: "rgba(0,0,0,0.5)" }}
          >
            <div
              className="h-full transition-all duration-300"
              style={{
                backgroundColor: "var(--accent)",
                width: `${pct}%`,
                boxShadow: "0 0 6px var(--accent)",
              }}
            />
          </div>
        )}
      </div>

      <div className="mt-2 px-0.5">
        <p
          className="text-sm font-medium truncate"
          style={{ color: "var(--text)" }}
        >
          {item.title}
        </p>
        {showType && typeLabel(item.type) && (
          <span
            className="text-xs px-1.5 py-0.5 rounded"
            style={{
              backgroundColor: "var(--surface-raised)",
              color: "var(--text-muted)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {typeLabel(item.type)}
          </span>
        )}
      </div>
    </Link>
    </>
  );
});
