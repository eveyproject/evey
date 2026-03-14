import { useState } from "react";
import { useStarRating, useRatingAggregate } from "@/hooks/useStarRating";

interface StarRatingProps {
  mediaId: string;
}

function StarIcon({ filled, half }: { filled: boolean; half?: boolean }) {
  if (half) {
    return (
      <svg className="w-5 h-5" viewBox="0 0 24 24" fill="none">
        <defs>
          <linearGradient id="half-fill">
            <stop offset="50%" stopColor="var(--accent)" />
            <stop offset="50%" stopColor="transparent" />
          </linearGradient>
        </defs>
        <path
          d="M12 2l3.09 6.26L22 9.27l-5 4.87L18.18 21 12 17.77 5.82 21 7 14.14 2 9.27l6.91-1.01L12 2z"
          fill="url(#half-fill)"
          stroke="var(--accent)"
          strokeWidth={1.5}
        />
      </svg>
    );
  }
  return (
    <svg className="w-5 h-5" viewBox="0 0 24 24">
      <path
        d="M12 2l3.09 6.26L22 9.27l-5 4.87L18.18 21 12 17.77 5.82 21 7 14.14 2 9.27l6.91-1.01L12 2z"
        fill={filled ? "var(--accent)" : "none"}
        stroke="var(--accent)"
        strokeWidth={1.5}
      />
    </svg>
  );
}

export function StarRating({ mediaId }: StarRatingProps) {
  const { rating, setRating } = useStarRating(mediaId);
  const { data: agg } = useRatingAggregate(mediaId);
  const [hovered, setHovered] = useState(0);

  const display = hovered > 0 ? hovered : rating;

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-1.5">
        <span className="text-sm" style={{ color: "var(--text-muted)" }}>
          Your rating:
        </span>
        <div
          className="flex items-center gap-0.5"
          onMouseLeave={() => setHovered(0)}
          aria-label={rating > 0 ? `Rated ${rating} out of 5 stars` : "Not rated"}
        >
          {[1, 2, 3, 4, 5].map((star) => (
            <button
              key={star}
              type="button"
              aria-label={`Rate ${star} star${star > 1 ? "s" : ""}`}
              onMouseEnter={() => setHovered(star)}
              onClick={() => setRating(star)}
              className="transition-transform hover:scale-110 focus:outline-none focus:scale-110 cursor-pointer"
              style={{ color: "var(--accent)" }}
            >
              <StarIcon filled={star <= display} />
            </button>
          ))}
        </div>
        {rating > 0 && (
          <span className="text-xs" style={{ color: "var(--text-muted)" }}>
            {rating}/5
          </span>
        )}
      </div>
      {agg && agg.count > 0 && (
        <span className="text-xs" style={{ color: "var(--text-muted)" }}>
          Community: {agg.average.toFixed(1)}/5 ({agg.count} {agg.count === 1 ? "rating" : "ratings"})
        </span>
      )}
    </div>
  );
}

/** Small read-only badge for MediaCard (shows star + number) */
export function StarRatingBadge({ mediaId }: { mediaId: string }) {
  const { rating } = useStarRating(mediaId);
  if (rating === 0) return null;
  return (
    <span
      className="absolute z-20 flex items-center gap-0.5 text-[10px] font-bold px-1 py-0.5 leading-none"
      style={{
        top: "0.5rem",
        left: "2.25rem",
        backgroundColor: "rgba(0,0,0,0.72)",
        color: "var(--accent)",
        borderRadius: "var(--radius-sm)",
      }}
      title={`Your rating: ${rating}/5`}
    >
      <svg className="w-2.5 h-2.5" viewBox="0 0 24 24" fill="currentColor">
        <path d="M12 2l3.09 6.26L22 9.27l-5 4.87L18.18 21 12 17.77 5.82 21 7 14.14 2 9.27l6.91-1.01L12 2z" />
      </svg>
      {rating}
    </span>
  );
}
