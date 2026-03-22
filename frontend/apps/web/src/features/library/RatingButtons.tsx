import { useTranslation } from "react-i18next";
import { ThumbsUp, ThumbsDown } from "lucide-react";
import { useMyRating, useSetRating } from "@/hooks/useRating";

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

  const btnStyle = (active: boolean, activeColor: string) => ({
    padding: "6px",
    borderRadius: "9999px",
    backgroundColor: active ? activeColor : "var(--btn-overlay-inactive-bg)",
    color: active ? "var(--overlay-text)" : "var(--overlay-text-muted)",
    backdropFilter: "blur(4px)",
    border: "none",
    cursor: "pointer",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    transition: "transform 0.15s, background-color 0.15s",
  });

  return (
    <div style={{ display: "flex", gap: "6px" }}>
      <button
        type="button"
        onClick={() => handleClick(1)}
        style={btnStyle(rating === 1, "var(--color-success)")}
        title={rating === 1 ? t("media.rating.removeLike") : t("media.rating.like")}
      >
        <ThumbsUp size={20} />
      </button>
      <button
        type="button"
        onClick={() => handleClick(-1)}
        style={btnStyle(rating === -1, "var(--color-danger)")}
        title={rating === -1 ? t("media.rating.removeDislike") : t("media.rating.dislike")}
      >
        <ThumbsDown size={20} />
      </button>
    </div>
  );
}
