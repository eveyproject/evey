import { useTranslation } from "react-i18next";
import { useMusicPlayerStore, type MusicTrack } from "@evey/store";
import { GripVertical, X } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { useDraggable, useDropZone, moveTargetIndex } from "@/lib/dnd";
import { addsTracks, isRowOf, type MusicDragData } from "./musicDrag";
import { useAddToQueue } from "./useAddToQueue";

// The player's queue as an editable list: a track dragged onto it lands where
// it is dropped, a row dragged within it moves, the handle moves a row with
// the arrow keys. Shown both on the music page and in the mini player, over
// the same store.
export const PLAYER_QUEUE_LIST = "player-queue";

export function QueueList({ zoneId, className = "" }: { zoneId: string; className?: string }) {
  const { t } = useTranslation();
  const queue = useMusicPlayerStore((s) => s.queue);
  const playing = useMusicPlayerStore((s) => s.playing);
  const source = useMusicPlayerStore((s) => s.source);
  const play = useMusicPlayerStore((s) => s.play);
  const moveInQueue = useMusicPlayerStore((s) => s.moveInQueue);
  const removeFromQueue = useMusicPlayerStore((s) => s.removeFromQueue);
  const addToQueue = useAddToQueue();

  const zone = useDropZone<MusicDragData>(
    zoneId,
    (d): d is MusicDragData => addsTracks(PLAYER_QUEUE_LIST)(d) || isRowOf(PLAYER_QUEUE_LIST)(d),
    (d, index) => {
      if (isRowOf(PLAYER_QUEUE_LIST)(d)) {
        moveInQueue(d.index, moveTargetIndex(d.index, index ?? queue.length));
        return;
      }
      void addToQueue(d, index);
    },
  );

  // Jumping keeps the queue and the source: play() without them would drop
  // the "back to album" link of the mini player.
  const jump = (track: MusicTrack) => {
    if (track.ID !== playing?.ID) play(track, queue, source);
  };

  return (
    <div
      ref={zone.ref}
      {...zone.props}
      data-testid={`queue-list-${zoneId}`}
      className={`relative rounded-[var(--radius-sm)] transition-colors-fast ${
        zone.over ? "outline outline-2 outline-[var(--accent)]" : zone.armed ? "outline outline-1 outline-dashed outline-[var(--accent)]" : ""
      } ${className}`}
    >
      {queue.length === 0 ? (
        <p className="text-xs text-[var(--text-muted)] text-center py-6 px-3">{t("media.music.queueDropHint")}</p>
      ) : (
        <ul>
          {queue.map((track, i) => (
            <QueueRow
              key={track.ID}
              track={track}
              index={i}
              count={queue.length}
              current={track.ID === playing?.ID}
              dropBefore={zone.index === i}
              dropAfter={zone.index === queue.length && i === queue.length - 1}
              onJump={() => jump(track)}
              onRemove={() => removeFromQueue(track.ID)}
              onMove={(to) => moveInQueue(i, to)}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function QueueRow({
  track,
  index,
  count,
  current,
  dropBefore,
  dropAfter,
  onJump,
  onRemove,
  onMove,
}: {
  track: MusicTrack;
  index: number;
  count: number;
  current: boolean;
  dropBefore: boolean;
  dropAfter: boolean;
  onJump: () => void;
  onRemove: () => void;
  onMove: (to: number) => void;
}) {
  const { t } = useTranslation();
  const drag = useDraggable(
    `${PLAYER_QUEUE_LIST}:${track.ID}`,
    (): MusicDragData => ({ kind: "music-row", list: PLAYER_QUEUE_LIST, index, track }),
    track.Title || "-",
  );
  return (
    <li
      data-drop-index={index}
      {...drag.props}
      className={`group flex items-center rounded-[var(--radius-sm)] hover:bg-[var(--surface-raised)] touch-manipulation ${current ? "bg-[var(--surface-raised)]" : ""} ${
        drag.dragging ? "opacity-40" : ""
      } ${dropBefore ? "shadow-[inset_0_2px_0_var(--accent)]" : ""} ${dropAfter ? "shadow-[inset_0_-2px_0_var(--accent)]" : ""}`}
    >
      <button
        type="button"
        aria-label={t("media.music.moveInQueue", { title: track.Title || "-" })}
        title={t("media.music.moveInQueueHint")}
        onKeyDown={(e) => {
          if (e.key === "ArrowUp" && index > 0) { e.preventDefault(); onMove(index - 1); }
          if (e.key === "ArrowDown" && index < count - 1) { e.preventDefault(); onMove(index + 1); }
        }}
        className="bg-transparent border-none cursor-grab text-[var(--text-muted)] opacity-50 group-hover:opacity-100 focus-visible:opacity-100 hover:text-[var(--text)] pl-1 pr-0.5 py-1 flex items-center shrink-0"
      >
        <GripVertical size={ICON.sm} aria-hidden="true" />
      </button>
      <button
        type="button"
        onClick={onJump}
        aria-current={current ? "true" : undefined}
        className="flex-1 min-w-0 text-left px-1.5 py-1 bg-transparent border-none cursor-pointer"
      >
        <p className={`text-sm leading-tight truncate ${current ? "font-semibold text-[var(--accent)]" : "text-[var(--text)]"}`}>{track.Title || "-"}</p>
        <p className="text-xs leading-tight truncate text-[var(--text-muted)]">{track.Artist || "-"}</p>
      </button>
      <button
        type="button"
        onClick={onRemove}
        className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] opacity-50 group-hover:opacity-100 focus-visible:opacity-100 hover:text-[var(--text)] transition-colors-fast p-1.5 flex items-center shrink-0"
        aria-label={t("media.music.removeFromQueue")}
        title={t("media.music.removeFromQueue")}
      >
        <X size={ICON.sm} aria-hidden="true" />
      </button>
    </li>
  );
}
