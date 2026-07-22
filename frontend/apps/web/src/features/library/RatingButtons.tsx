import { useTranslation } from "react-i18next";
import { ThumbsUp, ThumbsDown } from "lucide-react";
import { useMyRating, useSetRating } from "@/hooks/useRating";
import { ACTION_ICON_BOX } from "@/components/ui/Button";
import { ICON } from "@/constants/iconSizes";

interface RatingButtonsProps {
  mediaId: string;
}

export function RatingButtons({ mediaId }: RatingButtonsProps) {
  const { t } = useTranslation();
  const { data: rating = 0 } = useMyRating(mediaId);
  const setRating = useSetRating(mediaId);

  const handleClick = (value: number) => {
    setRating.mutate(rating === value ? 0 : value);
  };

  const btnClass = (active: boolean, activeBg: string) =>
    `${ACTION_ICON_BOX} rounded-full border-none cursor-pointer backdrop-blur-[4px] transition-[transform,background-color] duration-150 ${
      active
        ? `${activeBg} text-[var(--overlay-text)]`
        : "bg-[var(--btn-overlay-inactive-bg)] text-[var(--overlay-text-muted)]"
    }`;

  return (
    <div className="flex gap-1.5">
      <button
        type="button"
        onClick={() => handleClick(1)}
        className={btnClass(rating === 1, "bg-[var(--color-success)]")}
        title={rating === 1 ? t("media.rating.removeLike") : t("media.rating.like")}
        aria-label={rating === 1 ? t("media.rating.removeLike") : t("media.rating.like")}
      >
        <ThumbsUp size={ICON.xl} aria-hidden="true" />
      </button>
      <button
        type="button"
        onClick={() => handleClick(-1)}
        className={btnClass(rating === -1, "bg-[var(--color-danger)]")}
        title={rating === -1 ? t("media.rating.removeDislike") : t("media.rating.dislike")}
        aria-label={rating === -1 ? t("media.rating.removeDislike") : t("media.rating.dislike")}
      >
        <ThumbsDown size={ICON.xl} aria-hidden="true" />
      </button>
    </div>
  );
}
