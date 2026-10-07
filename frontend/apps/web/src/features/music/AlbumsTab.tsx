import { useImageToken } from "@/lib/imageToken";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Music, Download } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { MediaGrid } from "@/features/library/MediaGrid";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { startTokenDownload } from "@/utils/downloadToken";
import type { Album } from "./musicQueries";
import { coverArtSrc } from "../../lib/coverArtSrc";

export function AlbumsTab({
  albums,
  loading,
  isError,
  error,
  onRetry,
  libraryID,
  onAlbumClick,
}: {
  albums: Album[];
  loading: boolean;
  isError?: boolean;
  error?: unknown;
  onRetry?: () => void;
  libraryID: string | undefined;
  onAlbumClick: (a: Album) => void;
}) {
  const { t } = useTranslation();
  const accessToken = useImageToken();
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  // A failed album fetch must not collapse into "no albums found" - that reads
  // as an empty library when the server actually errored.
  if (!loading && isError) return <FetchErrorState error={error} onRetry={onRetry} />;
  if (!loading && !albums.length) return <p className="text-[var(--text-muted)]">{t("media.music.noAlbums")}</p>;

  async function downloadAlbum(a: Album) {
    if (!libraryID) return;
    // Click-time mint bound to the albumId (the server guard binds on
    // albumId+album, name empty here) - the JWT must not ride in the URL.
    try {
      await startTokenDownload(`/api/v1/music/albums/download?albumId=${encodeURIComponent(a.ID)}&libraryId=${encodeURIComponent(libraryID)}`, a.ID);
    } catch {
      toast.error(t("common.error"));
    }
  }

  return (
    <MediaGrid isLoading={loading}>
      {albums.map((a) => (
        <div
          key={a.ID}
          // Largeur FIXE, calée sur MediaCard (w-40 sm:w-48 = les minima des
          // pistes de MediaGrid). MediaGrid pose `justify-items-start` pour
          // aligner ces cartes à largeur fixe : sans largeur déclarée, la carte
          // se dimensionne sur son contenu, donc sur la longueur du titre, et la
          // pochette en aspect-square suivait - titre long = vignette géante,
          // titre court = vignette minuscule.
          className="evey-album-card relative w-40 sm:w-48 rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] border border-[var(--border)] hover:border-[var(--accent)] transition-colors-fast group"
        >
          <div
            role="button"
            tabIndex={0}
            onClick={() => onAlbumClick(a)}
            onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onAlbumClick(a); } }}
            className="cursor-pointer"
          >
            <div
              className="w-full flex items-center justify-center bg-[var(--surface)] bg-cover bg-center aspect-square"
              style={{
                backgroundImage: a.CoverArtURL ? `url(${coverArtSrc(a.CoverArtURL, tokenParam, 400)})` : undefined,
              }}
            >
              {!a.CoverArtURL && (
                <Music size={ICON["2xl"]} className="opacity-30 text-[var(--text-muted)]" />
              )}
            </div>
            <div className="p-2">
              <p className="font-medium text-sm truncate text-[var(--text)]">{a.Title || "-"}</p>
              <p className="text-xs truncate text-[var(--text-muted)]">{a.Artist || "-"}</p>
              <p className="text-xs text-[var(--text-muted)]">
                {t("media.music.trackCount", { count: a.TrackCount })}
                {a.Year > 0 ? ` · ${a.Year}` : ""}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); downloadAlbum(a); }}
            aria-label={t("media.music.downloadAlbum")}
            title={t("media.music.downloadAlbum")}
            className="absolute top-2 right-2 p-1.5 rounded-full bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--accent)] cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity-fast"
          >
            <Download size={ICON.sm} />
          </button>
        </div>
      ))}
    </MediaGrid>
  );
}
