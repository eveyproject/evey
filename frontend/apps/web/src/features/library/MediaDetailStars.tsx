import { Star } from "lucide-react";

export function Stars({ rating }: { rating: number }) {
  const full = Math.round(rating / 2);
  return (
    <span className="flex items-center gap-0.5">
      {Array.from({ length: 5 }, (_, i) => (
        <Star
          key={i}
          size={14}
          fill={i < full ? "var(--accent)" : "none"}
          stroke="var(--accent)"
          strokeWidth={1.5}
        />
      ))}
    </span>
  );
}
