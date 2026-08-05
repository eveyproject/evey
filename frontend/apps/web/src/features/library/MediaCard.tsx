import { memo, useRef, useState, useMemo, useEffect } from "react";
import { Star, Check, Play } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched, libraryServiceGetMediaItem, displayTitle } from "@evey/api";
import type { V1Artwork } from "@evey/api";
import { Blurhash } from "react-blurhash";
import { toast, useAuthStore } from "@evey/store";
import { FavoriteButton } from "./FavoriteButton";
import { useIsWatched } from "@/hooks/useWatchedMap";
import { typeLabel } from "@/utils/typeLabel";
import { HoverPreview } from "./MediaCardHoverPreview";
import { CardContextMenu } from "./MediaCardContextMenu";
import { invalidateWatchedState } from "@/lib/invalidateWatchedState";
import { artworkUrl } from "./artworkUrl";
import { PosterPlaceholder } from "./PosterPlaceholder";
import { ICON } from "@/constants/iconSizes";

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
  /** Drives the artwork cache-busting version (?v=) so a changed poster busts
   * the immutable cache. Absent on lite feed items (they pass posterSrc). */
  updatedAt?: string;
}

export interface MediaCardProps {
  item: MediaCardItem;
  href?: string;
  posterSrc?: string;
  /**
   * Explicit blurhash for the poster placeholder. Used by feed sliders that
   * receive a FeedItem (which carries primaryBlurhash inline) instead of a
   * full MediaItem with its artwork array. When omitted, the card falls
   * back to the primary/backdrop hash on item.artwork.
   */
  posterBlurhash?: string;
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
  onRequestAddToPlaylist?: (mediaId: string) => void;
  onRequestAddToCollection?: (mediaId: string) => void;
}

const formatResolution = (r: string) => (r === "2160p" ? "4K" : r);

export const MediaCard = memo(function MediaCard({
  item,
  href,
  posterSrc,
  posterBlurhash: posterBlurhashProp,
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
  onRequestAddToPlaylist,
  onRequestAddToCollection,
}: MediaCardProps) {
  const { t } = useTranslation();
  const [imgLoaded, setImgLoaded] = useState(false);
  const hoverTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [showPreview, setShowPreview] = useState(false);
  const profileId = useAuthStore((s) => s.profileId);
  const qc = useQueryClient();
  const invalidateItem = () => {
    invalidateWatchedState(qc, item.id);
  };
  const markWatched = usePlaybackServiceMarkAsWatched({ mutation: { onSuccess: () => { setWatchedLocal(true); toast.success(t("media.contextMenu.markedWatched")); invalidateItem(); } } });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({ mutation: { onSuccess: () => { setWatchedLocal(false); toast.success(t("media.contextMenu.markedUnwatched")); invalidateItem(); } } });

  const poster = useMemo(() => {
    // Always route through the backend endpoint when we know the media id -
    // it serves the local file when present, falls back to a 302 to the
    // external source only when local is missing. Using item.artwork[].url
    // directly leaks the raw provider URL (Amazon CDN for OMDb), which
    // Firefox's Opaque Response Blocking refuses to render as <img> due to
    // cross-origin opacity. The backend endpoint is same-origin, no ORB.
    return posterSrc
      ?? (item.id ? artworkUrl(item.id, "poster", item) : null);
  }, [posterSrc, item.id, item.updatedAt]);

  // Pull the primary artwork's blurhash (if any) for the placeholder. The
  // explicit posterBlurhashProp from a caller (e.g. feed slider) wins; we
  // fall back to primary → backdrop on item.artwork for detail pages with
  // a full MediaItem. Empty string when neither is present.
  const posterBlurhash = useMemo(() => {
    if (posterBlurhashProp) return posterBlurhashProp;
    const arts = (item.artwork ?? []) as V1Artwork[];
    const primary = arts.find((a) => a.type === "ARTWORK_TYPE_PRIMARY");
    if (primary?.blurhash) return primary.blurhash;
    const backdrop = arts.find((a) => a.type === "ARTWORK_TYPE_BACKDROP");
    return backdrop?.blurhash ?? "";
  }, [item.artwork, posterBlurhashProp]);

  const posterSrcSet = useMemo(() => {
    if (!poster) return undefined;
    const sep = poster.includes("?") ? "&" : "?";
    return `${poster}${sep}w=185 185w, ${poster}${sep}w=342 342w, ${poster}${sep}w=500 500w`;
  }, [poster]);

  const pct =
    progressPercent !== undefined
      ? Math.min(100, Math.max(0, Math.round(progressPercent)))
      : undefined;

  const watchedFromData = useIsWatched(item.id);
  const watchedFromProps = isWatched || watchedFromData || (pct !== undefined && pct >= WATCHED_THRESHOLD_PCT);
  const [watchedLocal, setWatchedLocal] = useState<boolean | null>(null);
  const watched = watchedLocal ?? watchedFromProps;

  const linkTo = href ?? "/media/$mediaId";
  const linkParams = href ? undefined : { mediaId: item.id ?? "" };

  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number } | null>(null);
  const menuBtnRef = useRef<HTMLButtonElement>(null);
  const hasPreviewData = !!(item.overview || item.communityRating || (item.genres && item.genres.length > 0));

  const prefetchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const longPressTriggered = useRef(false);

  // Clear pending hover/prefetch/long-press timers on unmount. In the virtualized
  // grid a card can unmount mid-hover (scrolled away) before mouseleave fires, so
  // the timers would otherwise call setShowPreview/prefetch after teardown.
  useEffect(
    () => () => {
      if (hoverTimer.current) clearTimeout(hoverTimer.current);
      if (prefetchTimer.current) clearTimeout(prefetchTimer.current);
      if (longPressTimer.current) clearTimeout(longPressTimer.current);
    },
    [],
  );

  const hasProgressBar = showProgress && pct !== undefined;

  return (
    <div className="relative w-fit">
    {ctxMenu && item.id && (
      <CardContextMenu
        mediaId={item.id}
        position={ctxMenu}
        onClose={() => setCtxMenu(null)}
        onAddToPlaylist={() => item.id && onRequestAddToPlaylist?.(item.id)}
        onAddToCollection={() => item.id && onRequestAddToCollection?.(item.id)}
      />
    )}
    <Link
      to={linkTo}
      params={linkParams as never}
      className={`evey-media-card group block flex-shrink-0 w-40 sm:w-48 relative${selected ? " ring-2 ring-[var(--accent)] rounded-[var(--radius-md)]" : ""}`}
      data-evey="media-card"
      data-testid="media-card"
      onMouseEnter={() => {
        if (item.id) {
          prefetchTimer.current = setTimeout(() => {
            qc.prefetchQuery({
              queryKey: [`/api/v1/media/${item.id}`],
              queryFn: ({ signal }) => libraryServiceGetMediaItem(item.id!, signal),
              staleTime: 60_000,
            });
          }, 100);
        }
        if (!hasPreviewData) return;
        hoverTimer.current = setTimeout(() => setShowPreview(true), 400);
      }}
      onMouseLeave={() => {
        if (prefetchTimer.current) clearTimeout(prefetchTimer.current);
        if (hoverTimer.current) clearTimeout(hoverTimer.current);
        setShowPreview(false);
      }}
      onFocus={() => {
        if (!hasPreviewData) return;
        hoverTimer.current = setTimeout(() => setShowPreview(true), 400);
      }}
      onBlur={() => {
        if (hoverTimer.current) clearTimeout(hoverTimer.current);
        setShowPreview(false);
      }}
      onContextMenu={(e) => {
        if (!item.id) return;
        e.preventDefault();
        setShowPreview(false);
        setCtxMenu({ x: e.clientX, y: e.clientY });
      }}
      onTouchStart={(e) => {
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
      }}
      onTouchEnd={(e) => {
        if (longPressTimer.current) clearTimeout(longPressTimer.current);
        if (longPressTriggered.current) e.preventDefault();
      }}
      onTouchMove={() => {
        if (longPressTimer.current) clearTimeout(longPressTimer.current);
      }}
      onClick={(e) => {
        if (longPressTriggered.current) { e.preventDefault(); longPressTriggered.current = false; return; }
        if (onSelect && selectionMode) {
          e.preventDefault();
          onSelect(item.id!, !selected);
        }
      }}
    >
      {showPreview && <HoverPreview item={item} />}

      <div
        className="evey-poster relative w-full overflow-hidden transition-all duration-200 ease-out group-hover:scale-[1.03] group-focus-within:scale-[1.03] aspect-[2/3] rounded-[var(--radius-md)] shadow-[var(--shadow-md)]"
      >
        {poster ? (
          <>
            {posterBlurhash && !imgLoaded ? (
              <div className="absolute inset-0">
                <Blurhash
                  hash={posterBlurhash}
                  width="100%"
                  height="100%"
                  resolutionX={32}
                  resolutionY={32}
                  punch={1}
                />
              </div>
            ) : null}
            <img
              src={poster}
              srcSet={posterSrcSet}
              sizes="(max-width: 640px) 185px, (max-width: 1024px) 342px, 500px"
              alt={displayTitle(item)}
              loading="lazy"
              decoding="async"
              className="w-full h-full object-cover transition-opacity-fast duration-300 relative"
              style={{ opacity: imgLoaded ? 1 : 0 }}
              onLoad={() => setImgLoaded(true)}
              onError={(e) => {
                const t = e.currentTarget;
                t.style.display = "none";
                const fb = t.nextElementSibling as HTMLElement | null;
                if (fb) fb.style.display = "flex";
              }}
            />
          </>
        ) : null}
        <PosterPlaceholder
          title={item.title}
          type={item.type}
          className="absolute inset-0"
          style={{ display: poster ? "none" : "flex" }}
        />

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


        {item.communityRating != null && item.communityRating > 0 && (
          <span
            className="absolute z-badge flex items-center gap-0.5 text-xs font-bold px-1 py-0.5 leading-none"
            style={{
              bottom: hasProgressBar ? "0.75rem" : "0.5rem",
              right: "0.5rem",
              backgroundColor: "var(--overlay-bg-strong)",
              color: "var(--badge-imdb-bg)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            <Star size={ICON.xs} fill="currentColor" />
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
                className="text-xs font-bold px-1.5 py-0.5 leading-none"
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
                className="text-xs font-bold px-1.5 py-0.5 leading-none"
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
            aria-label={selected ? t("common.aria.deselect") : t("common.aria.select")}
            onClick={(e) => { e.preventDefault(); e.stopPropagation(); onSelect?.(item.id!, !selected); }}
            className={`tap-hit absolute top-2 left-2 z-badge flex items-center justify-center w-5 h-5 rounded transition-colors-fast${selectionMode && !selected ? " opacity-0 group-hover:opacity-100 group-focus-within:opacity-100" : ""}`}
            style={{
              backgroundColor: selected ? "var(--accent)" : "var(--overlay-gradient)",
              border: `2px solid ${selected ? "var(--accent)" : "var(--overlay-text-muted)"}`,
            }}
          >
            {selected && (
              <Check size={ICON.xs} className="text-[var(--filter-active-text)]" strokeWidth={3}  aria-hidden="true" />
            )}
          </button>
        ) : (item.id && !lite) ? (
          <div className="absolute top-2 left-2 z-badge opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity-fast duration-200">
            <FavoriteButton mediaId={item.id} size="sm" />
          </div>
        ) : null}

        {item.id && (
          <button
            ref={menuBtnRef}
            aria-label={t("common.options")}
            aria-haspopup="true"
            aria-expanded={ctxMenu !== null}
            className="tap-hit absolute top-2 right-2 z-badge opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100 p-1 rounded bg-black/60 text-[var(--overlay-text)]"
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
            aria-label={t("common.play")}
            className="w-12 h-12 rounded-full flex items-center justify-center [@media(hover:hover)]:backdrop-blur-sm transition-transform duration-200 group-hover:scale-110 group-focus-within:scale-110 bg-[var(--glass-bg)] border border-[var(--glass-border)]"
          >
            <Play size={ICON.lg} className="text-[var(--overlay-text)] ml-0.5" fill="currentColor" aria-hidden="true" />
          </div>
          <p className="absolute bottom-2 left-2 right-2 text-xs font-medium text-[var(--overlay-text)] truncate text-center">
            {displayTitle(item)}
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
                borderRadius: "0 var(--radius-sm) var(--radius-sm) 0",
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
          {displayTitle(item)}
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
    {item.id && (
      <button
        type="button"
        className={`tap-hit absolute z-badge flex items-center justify-center w-6 h-6 rounded-full transition-colors-fast cursor-pointer ${
          watched
            ? "bg-[var(--color-success)] text-white"
            : "bg-black/60 text-white/60"
        }`}
        style={{ bottom: hasProgressBar ? "4.5rem" : "3.5rem", right: "0.5rem" }}
        data-testid="watched-badge"
        title={watched ? t("media.contextMenu.markUnwatched") : t("media.contextMenu.markWatched")}
        aria-label={watched ? t("media.contextMenu.markUnwatched") : t("media.contextMenu.markWatched")}
        onClick={() => {
          if (!profileId) return;
          if (watched) markUnwatched.mutate({ mediaItemId: item.id!, params: { profileId } });
          else markWatched.mutate({ mediaItemId: item.id!, params: { profileId } });
        }}
      >
        <Check size={ICON.sm} strokeWidth={3}  aria-hidden="true" />
      </button>
    )}
    </div>
  );
});
