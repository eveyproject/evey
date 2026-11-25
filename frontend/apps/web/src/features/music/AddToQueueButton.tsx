import { useTranslation } from "react-i18next";
import { ListPlus } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import type { MusicDragData } from "./musicDrag";
import { useAddToQueue } from "./useAddToQueue";

// The click twin of dragging onto the queue: same data, added at the end.
// `overlay` is the round button that appears over a cover on hover.
export function AddToQueueButton({ data, overlay = false }: { data: MusicDragData; overlay?: boolean }) {
  const { t } = useTranslation();
  const addToQueue = useAddToQueue();
  return (
    <button
      type="button"
      data-no-drag
      onClick={(e) => { e.stopPropagation(); void addToQueue(data); }}
      aria-label={t("media.music.addToQueue")}
      title={t("media.music.addToQueue")}
      className={
        overlay
          ? "absolute top-2 left-2 p-1.5 rounded-full bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--accent)] cursor-pointer opacity-0 group-hover:opacity-100 focus-visible:opacity-100 transition-opacity-fast inline-flex items-center"
          : "bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)] p-1 rounded inline-flex items-center"
      }
    >
      <ListPlus size={overlay ? ICON.sm : ICON.md} aria-hidden="true" />
    </button>
  );
}
