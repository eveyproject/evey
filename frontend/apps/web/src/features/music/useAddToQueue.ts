import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { toast, useMusicPlayerStore } from "@evey/store";
import { resolveDragTracks, type MusicDragData } from "./musicDrag";

/**
 * Adds tracks, an album or a playlist to the player's queue, at the end or at
 * a given position, and says what happened.
 */
export function useAddToQueue() {
  const { t } = useTranslation();
  const addToQueue = useMusicPlayerStore((s) => s.addToQueue);
  return useCallback(
    async (d: MusicDragData, at?: number | null) => {
      try {
        const tracks = await resolveDragTracks(d);
        const added = addToQueue(tracks, at ?? undefined);
        if (added > 0) toast.success(t("media.music.addedToQueue", { count: added }));
        else toast.info(t("media.music.alreadyInQueue"));
      } catch {
        toast.error(t("media.music.addToQueueFailed"));
      }
    },
    [addToQueue, t],
  );
}
