import { memo, useRef, useState, useEffect, useCallback } from "react";
import { createPortal } from "react-dom";
import { invalidateWatchedState } from "@/lib/invalidateWatchedState";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched } from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { useIsFavorite, useToggleFavorite } from "@/hooks/useFavorite";
import { useDemoLock } from "@/hooks/useDemoMode";

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
  const { lock } = useDemoLock();
  const ref = useRef<HTMLDivElement>(null);
  const [focusedIndex, setFocusedIndex] = useState(0);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const { data: isFav } = useIsFavorite(mediaId);
  const toggleFav = useToggleFavorite(mediaId);
  // Mirror MediaCard.invalidateItem: marking watched from the card's dedicated
  // button refreshes the item, the watch-history map (the green badge) and the
  // home feeds that filter watched items - the context-menu path must do the
  // same, else the badge stays grey and the item lingers in "Continue Watching".
  const invalidateWatched = () => {
    invalidateWatchedState(queryClient, mediaId);
  };
  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: () => {
        invalidateWatched();
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
        invalidateWatched();
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

  // Both the editor (mounted behind isAdmin in MediaDetailActions) and
  // AddCollectionItem (RoleAdmin) are out of reach for a viewer: offering them
  // led either to a page with no editor or to a picker whose every choice
  // failed. Same gate as the surfaces they lead to.
  const items = [
    ...(isAdmin
      ? [
          {
            key: "edit-metadata",
            label: t("media.contextMenu.editMetadata"),
            onClick: () => {
              navigate({ to: "/media/$mediaId", params: { mediaId }, search: { edit: true } as never });
              onClose();
            },
          },
        ]
      : []),
    {
      key: "add-to-playlist",
      label: t("media.contextMenu.addToPlaylist"),
      onClick: () => {
        onClose();
        onAddToPlaylist();
      },
    },
    ...(isAdmin
      ? [
          {
            key: "add-to-collection",
            label: t("media.contextMenu.addToCollection"),
            onClick: () => {
              onClose();
              onAddToCollection();
            },
          },
        ]
      : []),
    {
      key: isFav ? "remove-from-favorites" : "add-to-favorites",
      label: isFav ? t("media.contextMenu.removeFromFavorites") : t("media.contextMenu.addToFavorites"),
      onClick: () => {
        toggleFav.mutate(!!isFav);
        onClose();
      },
    },
    {
      key: "mark-watched",
      label: t("media.contextMenu.markWatched"),
      onClick: () => {
        if (!profileId) { toast.error(t("media.contextMenu.noProfileSelected")); onClose(); return; }
        markWatched.mutate({ mediaItemId: mediaId, params: { profileId } });
      },
    },
    {
      key: "mark-unwatched",
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

  // Rendered into <body>, not next to the card that opened it.
  //
  // Inline, the menu inherited the card's stacking context, so a card LATER in
  // the DOM painted over it - the menu opened underneath the neighbouring
  // posters and could not be clicked (user report, 2026-09-04). z-popover
  // cannot help there: a z-index only orders siblings within its own context.
  //
  // The same nesting also made `position: fixed` unreliable, because any
  // transformed ancestor (the grid uses hover scaling) makes `fixed` resolve
  // against that ancestor instead of the viewport - which is how a menu asked
  // to appear at the cursor ended up hundreds of pixels away.
  //
  // A portal fixes both at once and is what `fixed` + clientX/clientY already
  // assumed: coordinates relative to the viewport, painted above everything.
  return createPortal(
    <div
      ref={ref}
      role="menu"
      aria-label={t("common.actions")}
      className="fixed z-popover min-w-[160px] py-1 rounded-[var(--radius-md)] shadow-xl bg-[var(--surface)] border border-[var(--border)]"
      style={{
        left: position.x,
        top: position.y,
      }}
      onKeyDown={handleKeyDown}
    >
      {items.map((it, i) => (
        <button
          key={it.key}
          ref={(el) => { itemRefs.current[i] = el; }}
          role="menuitem"
          tabIndex={i === focusedIndex ? 0 : -1}
          onClick={it.onClick}
          {...(it.key === "edit-metadata" ? {} : lock)}
          onFocus={() => setFocusedIndex(i)}
          data-testid={`media-context-menu-${it.key}`}
          className="w-full text-left px-3 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
        >
          {it.label}
        </button>
      ))}
    </div>,
    document.body,
  );
});
