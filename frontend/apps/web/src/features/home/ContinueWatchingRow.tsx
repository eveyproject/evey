import { useFeedServiceContinueWatching } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { MediaCard } from "../library/MediaCard";

export function ContinueWatchingRow() {
  const { data, isLoading } = useFeedServiceContinueWatching();
  const items = data?.items ?? [];

  if (!isLoading && items.length === 0) return null;

  return (
    <FeedSection title="Continue Watching" isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showYear={false}
        />
      ))}
    </FeedSection>
  );
}
