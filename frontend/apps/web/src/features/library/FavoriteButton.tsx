import { Heart } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useIsFavorite, useToggleFavorite } from "@/hooks/useFavorite";

interface FavoriteButtonProps {
  mediaId: string;
  size?: "sm" | "md";
  className?: string;
}

export function FavoriteButton({ mediaId, size = "md", className = "" }: FavoriteButtonProps) {
  const { t } = useTranslation();
  const { data: isFav } = useIsFavorite(mediaId);
  const toggle = useToggleFavorite(mediaId);

  const iconPixels = size === "sm" ? 16 : 20;
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
      title={isFav ? t("media.contextMenu.removeFromFavorites") : t("media.contextMenu.addToFavorites")}
    >
      <Heart size={iconPixels} fill={isFav ? "currentColor" : "none"} strokeWidth={2} />
    </button>
  );
}
