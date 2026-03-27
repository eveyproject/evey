import { memo, useRef, useState, useEffect, useCallback, useMemo } from "react";
import { Star, Check, Play } from "lucide-react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched, libraryServiceGetMediaItem } from "@evey/api";
import type { V1Artwork } from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { FavoriteButton } from "./FavoriteButton";
import { StarRatingBadge } from "./StarRating";
import { useIsFavorite, useToggleFavorite } from "@/hooks/useFavorite";
import { AddToPlaylistModal } from "../playlist/AddToPlaylistModal";
import { AddToCollectionModal } from "../collection/AddToCollectionModal";
import { typeLabel } from "@/utils/typeLabel";

const WATCHED_THRESHOLD_PCT = 95;

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
  /** Skip per-card API calls (favorite, star rating) to reduce HTTP requests in lists */
  lite?: boolean;
}

export { typeLabel };

const formatResolution = (r: string) => (r === "2160p" ? "4K" : r);

const HoverPreview = memo(function HoverPreview({ item }: { item: MediaCardItem }) {
  const hasContent = item.overview || item.communityRating || (item.genres && item.genres.length > 0);
  if (!hasContent) return null;

  return (
    <div
      className="absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-56 rounded-lg p-3 shadow-xl z-overlay pointer-events-none transition-opacity-fast duration-200 hidden sm:block bg-[var(--surface)] border border-[var(--border)]"
    >
      <p className="text-sm font-semibold mb-1 truncate text-[var(--text)]">
        {item.title}
      </p>

      <div className="flex items-center gap-2 mb-1.5 text-xs text-[var(--text-muted)]">
        {item.year && <span>{item.year}</span>}
        {item.communityRating != null && item.communityRating > 0 && (
          <span className="flex items-center gap-0.5">
            <Star size={12} fill="currentColor" className="text-[var(--accent)]" />
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
              className="text-[10px] px-1.5 py-0.5 rounded-full bg-[var(--surface-raised)] text-[var(--text-muted)]"
            >
              {g}
            </span>
          ))}
        </div>
      )}

      {item.overview && (
        <p
          className="text-xs leading-relaxed line-clamp-3 text-[var(--text-secondary)]"
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
  const { t } = useTranslation();
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
        toast.success(t("media.contextMenu.markedWatched"));
        onClose();
      },
      onError: () => {
        toast.error(t("media.contextMenu.markError", { status: "watched" }));
        onClose();
      },
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaId}`] });
        toast.success(t("media.contextMenu.markedUnwatched"));
        onClose();
      },
      onError: () => {
        toast.error(t("media.contextMenu.markError", { status: "unwatched" }));
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
      label: t("media.contextMenu.editMetadata"),
      onClick: () => {
        navigate({ to: "/media/$mediaId", params: { mediaId }, search: { edit: true } as never });
        onClose();
      },
    },
    {
      label: t("media.contextMenu.addToPlaylist"),
      onClick: () => {
        onClose();
        onAddToPlaylist();
      },
    },
    {
      label: t("media.contextMenu.addToCollection"),
      onClick: () => {
        onClose();
        onAddToCollection();
      },
    },
    {
      label: isFav ? t("media.contextMenu.removeFromFavorites") : t("media.contextMenu.addToFavorites"),
      onClick: () => {
        toggleFav.mutate();
        onClose();
      },
    },
    {
      label: t("media.contextMenu.download"),
      onClick: () => {
        const tk = useAuthStore.getState().accessToken;
        window.open(`/api/v1/media/${mediaId}/download${tk ? `?token=${encodeURIComponent(tk)}` : ""}`, "_blank");
        onClose();
      },
    },
    {
      label: t("media.contextMenu.markWatched"),
      onClick: () => {
        if (!profileId) { toast.error(t("media.contextMenu.noProfileSelected")); onClose(); return; }
        markWatched.mutate({ mediaItemId: mediaId, params: { profileId } });
      },
    },
    {
      label: t("media.contextMenu.markUnwatched"),
      onClick: () => {
        if (!profileId) { toast.error(t("media.contextMenu.noProfileSelected")); onClose(); return; }
        markUnwatched.mutate({ mediaItemId: mediaId, params: { profileId } });
      },
    },
  ];

  return (
    <div
      ref={ref}
      className="fixed z-popover min-w-[160px] py-1 rounded-[var(--radius-md)] shadow-xl bg-[var(--surface)] border border-[var(--border)]"
      style={{
        left: position.x,
        top: position.y,
      }}
    >
      {items.map((it) => (
        <button
          key={it.label}
          onClick={it.onClick}
          className="w-full text-left px-3 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
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
  lite = false,
}: MediaCardProps) {
  const { t } = useTranslation();
  const [imgLoaded, setImgLoaded] = useState(false);
  const hoverTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [showPreview, setShowPreview] = useState(false);
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const qc = useQueryClient();
  const invalidateItem = useCallback(() => {
    if (item.id) void qc.invalidateQueries({ queryKey: [`/api/v1/media/${item.id}`] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/feeds/resume-playback"] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/playback"] });
  }, [item.id, qc]);
  const markWatched = usePlaybackServiceMarkAsWatched({ mutation: { onSuccess: invalidateItem } });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({ mutation: { onSuccess: invalidateItem } });

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

  const watched = isWatched || (pct !== undefined && pct >= WATCHED_THRESHOLD_PCT);

  const linkTo = href ?? "/media/$mediaId";
  const linkParams = href ? undefined : { mediaId: item.id ?? "" };

  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number } | null>(null);
  const [playlistModalOpen, setPlaylistModalOpen] = useState(false);
  const [collectionModalOpen, setCollectionModalOpen] = useState(false);
  const menuBtnRef = useRef<HTMLButtonElement>(null);
  const hasPreviewData = !!(item.overview || item.communityRating || (item.genres && item.genres.length > 0));

  const queryClient = useQueryClient();

  const prefetchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleMouseEnter = useCallback(() => {
    if (item.id) {
      prefetchTimer.current = setTimeout(() => {
        queryClient.prefetchQuery({
          queryKey: [`/api/v1/media/${item.id}`],
          queryFn: ({ signal }) => libraryServiceGetMediaItem(item.id!, signal),
          staleTime: 60_000,
        });
      }, 100);
    }
    if (!hasPreviewData) return;
    hoverTimer.current = setTimeout(() => setShowPreview(true), 400);
  }, [hasPreviewData, item.id, queryClient]);

  const handleMouseLeave = useCallback(() => {
    if (prefetchTimer.current) clearTimeout(prefetchTimer.current);
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

  // Long-press for mobile context menu
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const longPressTriggered = useRef(false);
  const handleTouchStart = useCallback((e: React.TouchEvent) => {
    if (!item.id) return;
    longPressTriggered.current = false;
    const touch = e.touches[0];
    const x = touch.clientX;
    const y = touch.clientY;
    longPressTimer.current = setTimeout(() => {
      longPressTriggered.current = true;
      setShowPreview(false);
      setCtxMenu({ x, y });
    }, 600);
  }, [item.id]);
  const handleTouchEnd = useCallback((e: React.TouchEvent) => {
    if (longPressTimer.current) clearTimeout(longPressTimer.current);
    if (longPressTriggered.current) e.preventDefault();
  }, []);
  const handleTouchMove = useCallback(() => {
    if (longPressTimer.current) clearTimeout(longPressTimer.current);
  }, []);

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
      className={`group block flex-shrink-0 w-36 sm:w-44 relative${selected ? " ring-2 ring-[var(--accent)] rounded-[var(--radius-md)]" : ""}`}
      data-testid="media-card"
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
      onFocus={handleFocus}
      onBlur={handleBlur}
      onContextMenu={handleContextMenu}
      onTouchStart={handleTouchStart}
      onTouchEnd={handleTouchEnd}
      onTouchMove={handleTouchMove}
      onClick={(e) => {
        if (longPressTriggered.current) { e.preventDefault(); longPressTriggered.current = false; return; }
        if (onSelect && selectionMode) { handleSelectionClick(e); }
      }}
    >
      {showPreview && <HoverPreview item={item} />}

      <div
        className="relative w-full overflow-hidden transition-all duration-200 ease-out group-hover:scale-[1.03] group-focus-within:scale-[1.03] aspect-[2/3] rounded-[var(--radius-md)] shadow-[var(--shadow-md)]"
      >
        {poster ? (
          <img
            src={poster}
            alt={item.title ?? ""}
            loading="lazy"
            className="w-full h-full object-cover transition-opacity-fast duration-300"
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
          className="absolute inset-0 items-center justify-center text-2xl font-bold bg-[var(--surface-raised)] text-[var(--text-muted)]"
          style={{
            display: poster ? "none" : "flex",
          }}
        >
          {initial}
        </div>

        {showYear && item.year !== undefined && (
          <span
            className="absolute top-2 right-2 text-xs font-medium px-1.5 py-0.5"
            style={{
              backgroundColor: "var(--overlay-bg-strong)",
              color: "var(--overlay-text)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {item.year}
          </span>
        )}

        {item.id && (
          <button
            type="button"
            className={`absolute z-badge bottom-2 right-2 flex items-center justify-center w-6 h-6 rounded-full backdrop-blur-[4px] transition-colors-fast ${
              watched
                ? "bg-[var(--color-success)] text-white"
                : "bg-black/40 text-white/50 opacity-0 group-hover:opacity-100"
            }`}
            data-testid="watched-badge"
            title={watched ? t("media.contextMenu.markUnwatched") : t("media.contextMenu.markWatched")}
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              if (!profileId) return;
              if (watched) markUnwatched.mutate({ mediaItemId: item.id!, params: { profileId } });
              else markWatched.mutate({ mediaItemId: item.id!, params: { profileId } });
            }}
          >
            <Check size={14} strokeWidth={3} />
          </button>
        )}

        {item.communityRating != null && item.communityRating > 0 && (
          <span
            className="absolute z-badge flex items-center gap-0.5 text-[10px] font-bold px-1 py-0.5 leading-none"
            style={{
              bottom: hasProgressBar ? "0.75rem" : "0.5rem",
              right: "0.5rem",
              backgroundColor: "var(--overlay-bg-strong)",
              color: "var(--badge-imdb-bg)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            <Star size={10} fill="currentColor" />
            {item.communityRating.toFixed(1)}
          </span>
        )}

        {(resolution || isHdr) && (
          <div
            className="absolute z-badge flex items-center gap-1"
            style={{ bottom: hasProgressBar ? "0.75rem" : "0.5rem", left: "0.5rem" }}
          >
            {resolution && (
              <span
                className="text-[10px] font-bold px-1.5 py-0.5 leading-none"
                style={{
                  backgroundColor: "var(--overlay-bg-strong)",
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
                  backgroundColor: "var(--badge-hdr-bg)",
                  color: "var(--badge-hdr-text)",
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
            className={`absolute top-2 left-2 z-badge flex items-center justify-center w-5 h-5 rounded transition-colors-fast${selectionMode && !selected ? " opacity-0 group-hover:opacity-100 group-focus-within:opacity-100" : ""}`}
            style={{
              backgroundColor: selected ? "var(--accent)" : "var(--overlay-gradient)",
              border: `2px solid ${selected ? "var(--accent)" : "var(--overlay-text-muted)"}`,
            }}
          >
            {selected && (
              <Check size={12} className="text-[var(--filter-active-text)]" strokeWidth={3} />
            )}
          </button>
        ) : (item.id && !lite) ? (
          <div className="absolute top-2 left-2 z-badge opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity-fast duration-200">
            <FavoriteButton mediaId={item.id} size="sm" />
          </div>
        ) : null}

        {item.id && !lite && <StarRatingBadge mediaId={item.id} />}

        {item.id && (
          <button
            ref={menuBtnRef}
            aria-label="Options"
            aria-haspopup="true"
            aria-expanded={ctxMenu !== null}
            className="absolute top-2 right-2 z-badge opacity-0 group-hover:opacity-100 focus:opacity-100 p-1 rounded bg-black/60 text-[var(--overlay-text)]"
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
          className="absolute inset-0 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity-fast duration-200"
          style={{ background: "linear-gradient(to top, var(--overlay-bg-strong) 0%, var(--overlay-border-subtle) 60%, transparent 100%)" }}
        >
          <div
            role="img"
            aria-label="Play"
            className="w-12 h-12 rounded-full flex items-center justify-center backdrop-blur-sm transition-transform duration-200 group-hover:scale-110 group-focus-within:scale-110 bg-[var(--glass-bg)] border border-[var(--glass-border)]"
          >
            <Play size={20} className="text-[var(--overlay-text)] ml-0.5" fill="currentColor" aria-hidden="true" />
          </div>
          <p className="absolute bottom-2 left-2 right-2 text-xs font-medium text-[var(--overlay-text)] truncate text-center">
            {item.title}
          </p>
        </div>

        {hasProgressBar && (
          <div
            className="absolute bottom-0 left-0 right-0 h-1.5 z-raised bg-[var(--overlay-bg)]"
          >
            <div
              className="h-full transition-all duration-300"
              style={{
                backgroundColor: "var(--accent)",
                width: `${pct}%`,
                borderRadius: "0 2px 2px 0",
                boxShadow: "0 0 8px var(--accent)",
              }}
            />
          </div>
        )}
      </div>

      <div className="mt-2 px-0.5">
        <p
          className="text-sm font-medium truncate text-[var(--text)]"
        >
          {item.title}
        </p>
        {showType && typeLabel(item.type) && (
          <span
            className="text-xs px-1.5 py-0.5 rounded-[var(--radius-sm)] bg-[var(--surface-raised)] text-[var(--text-muted)]"
          >
            {typeLabel(item.type)}
          </span>
        )}
      </div>
    </Link>
    </>
  );
});
