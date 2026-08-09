import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Virtuoso } from "react-virtuoso";
import { useAuthStore } from "@evey/store";
import { Music } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import type { Artist } from "./musicQueries";

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
  // Mirror the scroll-parent pattern from MediaGrid so Virtuoso's endReached
  // fires correctly inside AuthLayout's overflow-y-auto #main-content region.
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
  useEffect(() => { setScrollParent(document.getElementById("main-content")); }, []);

  if (loading) return <p className="text-[var(--text-muted)]">{t("media.music.loading")}</p>;
  // A failed artist fetch must not collapse into "no artists found" - that
  // reads as an empty library when the server actually errored.
  if (isError) return <FetchErrorState error={error} onRetry={onRetry} />;
  if (!artists.length) return <p className="text-[var(--text-muted)]">{t("media.music.noArtists")}</p>;

  return (
    <Virtuoso
      customScrollParent={scrollParent ?? undefined}
      totalCount={artists.length}
      overscan={400}
      itemContent={(index) => {
        const a = artists[index];
        return (
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
                style={{ backgroundImage: a.CoverArtURL ? `url(${a.CoverArtURL}${tokenParam})` : undefined }}
              >
                {!a.CoverArtURL && <Music size={ICON.lg} className="opacity-30 text-[var(--text-muted)]" />}
              </div>
              <span className="font-medium text-[var(--text)] truncate">{a.Name}</span>
            </div>
            <span className="text-sm text-[var(--text-muted)]">
              {t("media.music.albumCount", { count: a.AlbumCount })} · {t("media.music.trackCount", { count: a.TrackCount })}
            </span>
          </div>
        );
      }}
    />
  );
}
