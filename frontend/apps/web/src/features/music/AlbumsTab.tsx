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
import { useDraggable } from "@/lib/dnd";
import type { MusicDragData } from "./musicDrag";
import { AddToQueueButton } from "./AddToQueueButton";

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
        <AlbumCard
          key={a.ID}
          album={a}
          libraryID={libraryID}
          tokenParam={tokenParam}
          onOpen={() => onAlbumClick(a)}
          onDownload={() => void downloadAlbum(a)}
        />
      ))}
    </MediaGrid>
  );
}

// An album card: opens on click, drags onto the queue as a whole album.
function AlbumCard({
  album: a,
  libraryID,
  tokenParam,
  onOpen,
  onDownload,
}: {
  album: Album;
  libraryID: string | undefined;
  tokenParam: string;
  onOpen: () => void;
  onDownload: () => void;
}) {
  const { t } = useTranslation();
  const drag = useDraggable(
    `album:${a.ID}`,
    (): MusicDragData => ({ kind: "music-album", album: a, libraryID: libraryID ?? "" }),
    a.Title || "-",
    !libraryID,
  );
  return (
    <div
      {...drag.props}
      // Largeur FIXE, calée sur MediaCard (w-40 sm:w-48 = les minima des
      // pistes de MediaGrid). MediaGrid pose `justify-items-start` pour
      // aligner ces cartes à largeur fixe : sans largeur déclarée, la carte
      // se dimensionne sur son contenu, donc sur la longueur du titre, et la
      // pochette en aspect-square suivait - titre long = vignette géante,
      // titre court = vignette minuscule.
      className={`evey-album-card relative w-40 sm:w-48 rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] border border-[var(--border)] hover:border-[var(--accent)] transition-colors-fast group ${drag.dragging ? "opacity-40" : ""}`}
    >
      <div
        role="button"
        tabIndex={0}
        onClick={onOpen}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onOpen(); } }}
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
        data-no-drag
        onClick={(e) => { e.stopPropagation(); onDownload(); }}
        aria-label={t("media.music.downloadAlbum")}
        title={t("media.music.downloadAlbum")}
        className="absolute top-2 right-2 p-1.5 rounded-full bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--accent)] cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity-fast"
      >
        <Download size={ICON.sm} />
      </button>
      {libraryID && (
        <AddToQueueButton overlay data={{ kind: "music-album", album: a, libraryID }} />
      )}
    </div>
  );
}
