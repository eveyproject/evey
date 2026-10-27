import { memo, type ReactNode } from "react";
import { POSTER_RATIO, useImageRetry } from "@evey/client";
import { Pressable } from "./ui";
import { Icon } from "./icons";

// A card's picture: a failed load is retried before the fallback shows (see
// useImageRetry; the Android television's PosterImage does the same).
function CardImage({ url, fallback }: { url?: string; fallback: ReactNode }) {
  const { src, onError, onLoad } = useImageRetry(url);
  if (!src) return <>{fallback}</>;
  return <img src={src} alt="" loading="lazy" onError={onError} onLoad={onLoad} />;
}

interface PosterCardProps {
  title: string;
  subtitle?: string;
  imageUrl?: string;
  /** Card width in dp: CARD_WIDTH_FEATURED on the home, CARD_WIDTH_GRID in a grid. */
  width: number;
  aspect?: "poster" | "square";
  rating?: number;
  /** How far in, 0 to 100: the bar under the picture. */
  percent?: number;
  onPress: () => void;
  focusKey?: string;
  autoFocus?: boolean;
  onFocus?: () => void;
}

/** The Android television's poster card; the ring and the 5 % lift are FocusableCard's. */
function PosterCardImpl({ title, subtitle, imageUrl, width, aspect = "poster", rating, percent, onPress, focusKey, autoFocus, onFocus }: PosterCardProps) {
  const w = width / 10;
  const h = aspect === "poster" ? w * POSTER_RATIO : w;
  return (
    <Pressable className="tv-card poster-card" onPress={onPress} focusKey={focusKey} autoFocus={autoFocus} onFocus={onFocus} label={title}>
      <span className="poster" style={{ width: `${w}rem`, height: `${h}rem` }}>
        <CardImage url={imageUrl} fallback={<span className="poster-initial">{title.trim().charAt(0).toUpperCase() || "?"}</span>} />
        {rating != null && rating > 0 ? (
          <span className="rating-badge"><Icon name="star" size={12} />{rating.toFixed(1)}</span>
        ) : null}
      </span>
      {percent != null && percent > 0 ? (
        <span className="card-progress"><i style={{ width: `${Math.min(100, percent)}%` }} /></span>
      ) : null}
      <span className="card-title" style={{ width: `${w}rem` }}>{title}</span>
      {subtitle ? <span className="card-sub" style={{ width: `${w}rem` }}>{subtitle}</span> : null}
    </Pressable>
  );
}

export const PosterCard = memo(PosterCardImpl);

interface GridCardProps {
  title: string;
  subtitle?: string;
  imageUrl?: string;
  rating?: number;
  resolution?: string | null;
  hdr?: boolean;
  /** undefined: no watched mark at all (an item without id). */
  watched?: boolean;
  percent?: number;
  onPress: () => void;
  focusKey?: string;
  autoFocus?: boolean;
  onFocus?: () => void;
}

/** A library grid card: the poster with its technical badges, the title under it. */
function GridCardImpl({ title, subtitle, imageUrl, rating, resolution, hdr, watched, percent, onPress, focusKey, autoFocus, onFocus }: GridCardProps) {
  const hasProgress = percent != null && percent > 0;
  const low = hasProgress ? "raised" : "";
  return (
    <Pressable className="tv-card grid-card" onPress={onPress} focusKey={focusKey} autoFocus={autoFocus} onFocus={onFocus} label={title}>
      <span className="poster grid-poster">
        <CardImage url={imageUrl} fallback={<span className="muted">?</span>} />
        {rating != null && rating > 0 ? (
          <span className={`tech-badge rating ${low}`}><Icon name="star" size={11} />{rating.toFixed(1)}</span>
        ) : null}
        {resolution || hdr ? (
          <span className={`tech-row ${low}`}>
            {resolution ? <span className="tech-badge">{resolution}</span> : null}
            {hdr ? <span className="tech-badge hdr">HDR</span> : null}
          </span>
        ) : null}
        {watched !== undefined ? (
          <span className={`watched-mark ${watched ? "on" : ""}`}><Icon name="checkmark" size={12} /></span>
        ) : null}
        {hasProgress ? <span className="poster-progress"><i style={{ width: `${Math.min(100, Math.max(0, percent))}%` }} /></span> : null}
      </span>
      <span className="grid-title">{title}</span>
      {subtitle ? <span className="grid-sub">{subtitle}</span> : null}
    </Pressable>
  );
}

export const GridCard = memo(GridCardImpl);
