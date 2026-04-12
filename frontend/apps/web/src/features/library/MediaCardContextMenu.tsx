import { memo, useRef, useState, useEffect, useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched } from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { useIsFavorite, useToggleFavorite } from "@/hooks/useFavorite";

export const CardContextMenu = memo(function CardContextMenu({
  mediaId,
  onClose,
  position,
  onAddToPlaylist,
  onAddToCollection,
}: {
  mediaId: string;
  onClose: () => void;
  position: { x: number; y: number };
  onAddToPlaylist: () => void;
  onAddToCollection: () => void;
}) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  const [focusedIndex, setFocusedIndex] = useState(0);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId);
  const { data: isFav } = useIsFavorite(mediaId);
  const toggleFav = useToggleFavorite(mediaId);
  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaId}`] });
        toast.success(t("media.contextMenu.markedWatched"));
        onClose();
      },
      onError: () => {
        toast.error(t("media.contextMenu.markError", { status: "watched" }));
        onClose();
      },
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${mediaId}`] });
        toast.success(t("media.contextMenu.markedUnwatched"));
        onClose();
      },
      onError: () => {
        toast.error(t("media.contextMenu.markError", { status: "unwatched" }));
        onClose();
      },
    },
  });

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [onClose]);

  const items = [
    {
      label: t("media.contextMenu.editMetadata"),
      onClick: () => {
        navigate({ to: "/media/$mediaId", params: { mediaId }, search: { edit: true } as never });
        onClose();
      },
    },
    {
      label: t("media.contextMenu.addToPlaylist"),
      onClick: () => {
        onClose();
        onAddToPlaylist();
      },
    },
    {
      label: t("media.contextMenu.addToCollection"),
      onClick: () => {
        onClose();
        onAddToCollection();
      },
    },
    {
      label: isFav ? t("media.contextMenu.removeFromFavorites") : t("media.contextMenu.addToFavorites"),
      onClick: () => {
        toggleFav.mutate(!!isFav);
        onClose();
      },
    },
    {
      label: t("media.contextMenu.download"),
      onClick: () => {
        const tk = useAuthStore.getState().accessToken;
        window.open(`/api/v1/media/${mediaId}/download${tk ? `?token=${encodeURIComponent(tk)}` : ""}`, "_blank");
        onClose();
      },
    },
    {
      label: t("media.contextMenu.markWatched"),
      onClick: () => {
        if (!profileId) { toast.error(t("media.contextMenu.noProfileSelected")); onClose(); return; }
        markWatched.mutate({ mediaItemId: mediaId, params: { profileId } });
      },
    },
    {
      label: t("media.contextMenu.markUnwatched"),
      onClick: () => {
        if (!profileId) { toast.error(t("media.contextMenu.noProfileSelected")); onClose(); return; }
        markUnwatched.mutate({ mediaItemId: mediaId, params: { profileId } });
      },
    },
  ];

  useEffect(() => {
    itemRefs.current[0]?.focus();
  }, []);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    switch (e.key) {
      case "ArrowDown": {
        e.preventDefault();
        const next = (focusedIndex + 1) % items.length;
        setFocusedIndex(next);
        itemRefs.current[next]?.focus();
        break;
      }
      case "ArrowUp": {
        e.preventDefault();
        const prev = (focusedIndex - 1 + items.length) % items.length;
        setFocusedIndex(prev);
        itemRefs.current[prev]?.focus();
        break;
      }
      case "Home": {
        e.preventDefault();
        setFocusedIndex(0);
        itemRefs.current[0]?.focus();
        break;
      }
      case "End": {
        e.preventDefault();
        const last = items.length - 1;
        setFocusedIndex(last);
        itemRefs.current[last]?.focus();
        break;
      }
      case "Escape":
        e.preventDefault();
        onClose();
        break;
      case "Tab":
        e.preventDefault();
        break;
    }
  }, [focusedIndex, items.length, onClose]);

  return (
    <div
      ref={ref}
      role="menu"
      aria-label={t("media.contextMenu.editMetadata")}
      className="fixed z-popover min-w-[160px] py-1 rounded-[var(--radius-md)] shadow-xl bg-[var(--surface)] border border-[var(--border)]"
      style={{
        left: position.x,
        top: position.y,
      }}
      onKeyDown={handleKeyDown}
    >
      {items.map((it, i) => (
        <button
          key={it.label}
          ref={(el) => { itemRefs.current[i] = el; }}
          role="menuitem"
          tabIndex={i === focusedIndex ? 0 : -1}
          onClick={it.onClick}
          onFocus={() => setFocusedIndex(i)}
          className="w-full text-left px-3 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
        >
          {it.label}
        </button>
      ))}
    </div>
  );
});
