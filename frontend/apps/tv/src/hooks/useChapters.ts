import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface Chapter {
  title: string;
  startTimeMs: number;
  endTimeMs: number;
}

export function useChapters(mediaId: string | undefined) {
  return useQuery<Chapter[]>({
    queryKey: ["/api/v1/media/chapters", mediaId],
    queryFn: async () => {
      try {
        const data = await eveyFetch<{ chapters: Chapter[] }>({
          url: `/api/v1/media/${mediaId}/chapters`,
          method: "GET",
        });
        return data.chapters ?? [];
      } catch {
        return [];
      }
    },
    enabled: !!mediaId,
    staleTime: Infinity,
  });
}

// The chapter whose [startTimeMs, nextStartTimeMs) window contains the current
// position - or -1 before the first chapter or once the list is empty. Shared by
// both the picker (highlights the active row) and any future progress-bar label.
export function findCurrentChapterIndex(chapters: Chapter[], currentTimeMs: number): number {
  for (let i = 0; i < chapters.length; i++) {
    const start = chapters[i].startTimeMs;
    const end = i === chapters.length - 1 ? Infinity : chapters[i + 1].startTimeMs;
    if (currentTimeMs >= start && currentTimeMs < end) return i;
  }
  return -1;
}
