import { useIsFavorite, useToggleFavorite } from "@/hooks/useFavorite";

interface FavoriteButtonProps {
  mediaId: string;
  size?: "sm" | "md";
  className?: string;
}

export function FavoriteButton({ mediaId, size = "md", className = "" }: FavoriteButtonProps) {
  const { data: isFav } = useIsFavorite(mediaId);
  const toggle = useToggleFavorite(mediaId);

  const iconSize = size === "sm" ? "w-4 h-4" : "w-5 h-5";
  const padding = size === "sm" ? "p-1" : "p-1.5";

  return (
    <button
      type="button"
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        toggle.mutate();
      }}
      className={`${padding} rounded-full transition-all duration-200 hover:scale-110 ${className}`}
      style={{
        backgroundColor: "rgba(0,0,0,0.5)",
        color: isFav ? "#ef4444" : "rgba(255,255,255,0.8)",
        backdropFilter: "blur(4px)",
      }}
      title={isFav ? "Remove from favorites" : "Add to favorites"}
    >
      <svg className={iconSize} viewBox="0 0 24 24" fill={isFav ? "currentColor" : "none"} stroke="currentColor" strokeWidth={2}>
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z"
        />
      </svg>
    </button>
  );
}
