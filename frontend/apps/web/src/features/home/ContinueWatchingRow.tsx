import { useMemo } from "react";
import { usePlaybackServiceGetResumeItems } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { MediaCard } from "../library/MediaCard";

export function ContinueWatchingRow() {
  const { data, isLoading } = usePlaybackServiceGetResumeItems();
  const items = data?.items ?? [];
  const progress = data?.progress ?? [];

  const progressMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const p of progress) {
      if (p.mediaItemId && p.percentComplete !== undefined) {
        map.set(p.mediaItemId, p.percentComplete);
      }
    }
    return map;
  }, [progress]);

  if (!isLoading && items.length === 0) return null;

  return (
    <FeedSection title="Continue Watching" isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.id}
          item={item}
          showYear={false}
          showProgress
          progressPercent={progressMap.get(item.id ?? "")}
        />
      ))}
    </FeedSection>
  );
}
