import { useTranslation } from "react-i18next";
import { ThumbsUp, ThumbsDown } from "lucide-react";
import { useMyRating, useSetRating } from "@/hooks/useRating";
import { ACTION_ICON_BOX } from "@/components/ui/Button";
import { ICON } from "@/constants/iconSizes";
import { useDemoLock } from "@/hooks/useDemoMode";

interface RatingButtonsProps {
  mediaId: string;
}

export function RatingButtons({ mediaId }: RatingButtonsProps) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const { data: rating = 0, isError } = useMyRating(mediaId);
  const setRating = useSetRating(mediaId);

  // A failed read leaves `data` undefined, so the thumbs fall back to neutral -
  // and neutral is a STATEMENT ("you have no opinion on this"), not an absence.
  // Clicking from there writes that statement over whatever the viewer had
  // actually recorded, which they were never shown. The server stopped
  // pretending on 2026-08-24 (Internal instead of a quiet value: 0); this is
  // the screen catching up. Disabled says "not now" without inventing a third
  // look for "unknown".
  const unknown = isError;

  const handleClick = (value: number) => {
    if (unknown) return;
    setRating.mutate(rating === value ? 0 : value);
  };

  const btnClass = (active: boolean, activeBg: string) =>
    `${ACTION_ICON_BOX} rounded-full border-none backdrop-blur-[4px] transition-[transform,background-color] duration-150 ${
      unknown ? "cursor-not-allowed opacity-50 " : "cursor-pointer "
    }${
      active
        ? `${activeBg} text-[var(--overlay-text)]`
        : "bg-[var(--btn-overlay-inactive-bg)] text-[var(--overlay-text-muted)]"
    }`;

  return (
    <div className="flex gap-1.5">
      <button
        {...lock}
        type="button"
        onClick={() => handleClick(1)}
        disabled={!!lock.disabled || (unknown)}
        className={btnClass(rating === 1, "bg-[var(--color-success)]")}
        title={rating === 1 ? t("media.rating.removeLike") : t("media.rating.like")}
        aria-label={rating === 1 ? t("media.rating.removeLike") : t("media.rating.like")}
      >
        <ThumbsUp size={ICON.xl} aria-hidden="true" />
      </button>
      <button
        {...lock}
        type="button"
        onClick={() => handleClick(-1)}
        disabled={!!lock.disabled || (unknown)}
        className={btnClass(rating === -1, "bg-[var(--color-danger)]")}
        title={rating === -1 ? t("media.rating.removeDislike") : t("media.rating.dislike")}
        aria-label={rating === -1 ? t("media.rating.removeDislike") : t("media.rating.dislike")}
      >
        <ThumbsDown size={ICON.xl} aria-hidden="true" />
      </button>
    </div>
  );
}
