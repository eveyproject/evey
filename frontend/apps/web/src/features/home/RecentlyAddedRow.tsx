import { useFeedServiceRecentlyAdded } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { FeedItemCard } from "./FeedItemCard";

export function RecentlyAddedRow() {
  const { data, isLoading } = useFeedServiceRecentlyAdded();
  const items = data?.items ?? [];

  if (!isLoading && items.length === 0) return null;

  return (
    <FeedSection title="Recently Added" isLoading={isLoading}>
      {items.map((item) => (
        <FeedItemCard key={item.mediaItemId} item={item} />
      ))}
    </FeedSection>
  );
}
