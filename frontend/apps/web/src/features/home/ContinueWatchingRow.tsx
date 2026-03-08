import { useFeedServiceContinueWatching } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { FeedItemCard } from "./FeedItemCard";

export function ContinueWatchingRow() {
  const { data, isLoading } = useFeedServiceContinueWatching();
  const items = data?.items ?? [];

  if (!isLoading && items.length === 0) return null;

  return (
    <FeedSection title="Continue Watching" isLoading={isLoading}>
      {items.map((item) => (
        <FeedItemCard key={item.mediaItemId} item={item} showProgress />
      ))}
    </FeedSection>
  );
}
