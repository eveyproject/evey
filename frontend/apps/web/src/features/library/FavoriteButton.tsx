import { Heart } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useIsFavorite, useToggleFavorite } from "@/hooks/useFavorite";
import { ACTION_ICON_BOX } from "@/components/ui/Button";
import { ICON } from "@/constants/iconSizes";
import { useDemoLock } from "@/hooks/useDemoMode";

interface FavoriteButtonProps {
  mediaId: string;
  size?: "sm" | "md" | "lg";
  className?: string;
}

export function FavoriteButton({ mediaId, size = "md", className = "" }: FavoriteButtonProps) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const { data: isFav } = useIsFavorite(mediaId);
  const toggle = useToggleFavorite(mediaId);

  const iconPixels = size === "sm" ? ICON.md : size === "lg" ? ICON.xl : ICON.lg;
  // "lg" (the media action row) takes the shared square box so it renders as a
  // circle at the row's height; the smaller sizes stay padding-based, sitting
  // over artwork on cards where there is no row to line up with.
  const box = size === "sm" ? "p-1" : size === "lg" ? ACTION_ICON_BOX : "p-1.5";

  return (
    <button
      {...lock}
      type="button"
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        toggle.mutate(!!isFav);
      }}
      className={`${box} rounded-full transition-all duration-200 hover:scale-110 ${className}`}
      style={{
        backgroundColor: "var(--overlay-bg)",
        color: isFav ? "var(--color-danger)" : "var(--overlay-text-muted)",
        backdropFilter: "blur(4px)",
      }}
      title={isFav ? t("media.contextMenu.removeFromFavorites") : t("media.contextMenu.addToFavorites")}
      aria-label={isFav ? t("media.contextMenu.removeFromFavorites") : t("media.contextMenu.addToFavorites")}
      data-testid={isFav ? "favorite-btn-active" : "favorite-btn"}
    >
      <Heart size={iconPixels} fill={isFav ? "currentColor" : "none"} strokeWidth={2} aria-hidden="true" />
    </button>
  );
}
