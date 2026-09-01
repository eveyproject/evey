import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useAuthStore } from "@evey/store";
import { Music } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import type { Artist } from "./musicQueries";
import { coverArtSrc } from "./coverArtSrc";

// Rows have no wrapping text (name is truncated to one line) and the avatar is
// a fixed size, so the row height never varies with content - a constant
// estimate is exact. 48px avatar + 2*12px vertical padding + 1px*2 border +
// 4px bottom margin.
const ROW_HEIGHT = 78;

export function ArtistsTab({
  artists,
  loading,
  isError,
  error,
  onRetry,
  onArtistClick,
}: {
  artists: Artist[];
  loading: boolean;
  isError?: boolean;
  error?: unknown;
  onRetry?: () => void;
  onArtistClick: (a: Artist) => void;
}) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const parentRef = useRef<HTMLDivElement>(null);
  // Mirror the scroll-parent pattern from MediaGrid so the virtualizer's
  // getScrollElement resolves AuthLayout's overflow-y-auto #main-content
  // region rather than this component's own (non-scrolling) root.
  const [scrollEl, setScrollEl] = useState<HTMLElement | null>(null);
  useEffect(() => { setScrollEl(document.getElementById("main-content")); }, []);

  const virtualizer = useVirtualizer({
    count: artists.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
    scrollMargin:
      parentRef.current && scrollEl
        ? parentRef.current.getBoundingClientRect().top - scrollEl.getBoundingClientRect().top + scrollEl.scrollTop
        : 0,
  });

  if (loading) return <p className="text-[var(--text-muted)]">{t("media.music.loading")}</p>;
  // A failed artist fetch must not collapse into "no artists found" - that
  // reads as an empty library when the server actually errored.
  if (isError) return <FetchErrorState error={error} onRetry={onRetry} />;
  if (!artists.length) return <p className="text-[var(--text-muted)]">{t("media.music.noArtists")}</p>;

  return (
    <div ref={parentRef} style={{ height: virtualizer.getTotalSize(), width: "100%", position: "relative" }}>
      {virtualizer.getVirtualItems().map((vItem) => {
        const a = artists[vItem.index];
        return (
          <div
            key={vItem.key}
            data-index={vItem.index}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              width: "100%",
              transform: `translateY(${vItem.start - virtualizer.options.scrollMargin}px)`,
            }}
          >
            <div
              onClick={() => onArtistClick(a)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onArtistClick(a); } }}
              className="flex items-center justify-between px-4 py-3 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] cursor-pointer hover:border-[var(--accent)] transition-colors-fast mb-1"
            >
              <div className="flex items-center gap-3 min-w-0">
                <div
                  className="w-12 h-12 rounded-full bg-cover bg-center bg-[var(--surface)] flex items-center justify-center flex-shrink-0"
                  style={{ backgroundImage: a.CoverArtURL ? `url(${coverArtSrc(a.CoverArtURL, tokenParam, 96)})` : undefined }}
                >
                  {!a.CoverArtURL && <Music size={ICON.lg} className="opacity-30 text-[var(--text-muted)]" />}
                </div>
                <span className="font-medium text-[var(--text)] truncate">{a.Name}</span>
              </div>
              <span className="text-sm text-[var(--text-muted)]">
                {t("media.music.albumCount", { count: a.AlbumCount })} · {t("media.music.trackCount", { count: a.TrackCount })}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}
