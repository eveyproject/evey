import { useQuery } from "@tanstack/react-query";
import { mKVServiceListChapters } from "@evey/api";
import { skippableFromChapters, type ResolvedSegment } from "../lib/skipSegments";

const NONE: ResolvedSegment[] = [];

/**
 * The skippable parts a file's own MKV chapters name ("Opening", "Recap",
 * "Ending"...), the fallback while the server has not detected segments yet.
 * Asked only when `needed` (no server segment for this title) and a file path
 * is known. The phone and the web had it each in their own copy; both
 * televisions had no fallback at all until 2026-09-24.
 */
export function useChapterSkippables(filePath: string | undefined, needed: boolean): ResolvedSegment[] {
  const { data } = useQuery({
    queryKey: ["/api/v1/mkv/chapters", filePath],
    queryFn: ({ signal }) => mKVServiceListChapters({ path: filePath }, signal),
    enabled: !!filePath && needed,
    staleTime: Infinity,
  });
  return needed && data?.chapters ? skippableFromChapters(data.chapters) : NONE;
}
