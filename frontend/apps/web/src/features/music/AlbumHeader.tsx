import { useTranslation } from "react-i18next";
import { Music, Play, Shuffle } from "lucide-react";
import { useImageToken } from "@/lib/imageToken";
import { formatDurationSec } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { ICON } from "@/constants/iconSizes";
import { coverArtSrc } from "../../lib/coverArtSrc";
import type { Album } from "./musicQueries";

// The head of an open album: its cover, its name, who and when, how long, and
// the two ways to start it. Opening an album used to land on the bare track
// table with a filter chip, a page far poorer than a film's or a series'.
export function AlbumHeader({
  album,
  onPlay,
  onShuffle,
}: {
  album: Album;
  onPlay: () => void;
  onShuffle: () => void;
}) {
  const { t } = useTranslation();
  const accessToken = useImageToken();
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const cover = coverArtSrc(album.CoverArtURL, tokenParam, 400);
  const facts = [
    album.Artist,
    album.Year > 0 ? String(album.Year) : "",
    t("media.music.trackCount", { count: album.TrackCount }),
    formatDurationSec(album.Duration),
  ].filter(Boolean);

  return (
    <div className="flex flex-col sm:flex-row gap-6 items-start sm:items-end mb-6" data-testid="album-header">
      <div
        className="w-40 sm:w-48 aspect-square shrink-0 rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface)] bg-cover bg-center flex items-center justify-center shadow-lg border border-[var(--border)]"
        style={{ backgroundImage: cover ? `url(${cover})` : undefined }}
        data-testid="album-header-cover"
      >
        {!cover && <Music size={ICON["2xl"]} className="opacity-30 text-[var(--text-muted)]" />}
      </div>
      <div className="min-w-0 space-y-3">
        <h2 className="text-3xl font-bold text-[var(--text)] text-balance">{album.Title || "-"}</h2>
        <p className="text-sm text-[var(--text-muted)]">{facts.join(" · ")}</p>
        <div className="flex gap-3 flex-wrap">
          <Button variant="primary" size="lg" onClick={onPlay} data-testid="album-header-play">
            <Play size={ICON.md} fill="currentColor" />
            {t("media.detail.play")}
          </Button>
          <Button variant="secondary" size="lg" onClick={onShuffle} data-testid="album-header-shuffle">
            <Shuffle size={ICON.md} />
            {t("media.music.shuffle")}
          </Button>
        </div>
      </div>
    </div>
  );
}
