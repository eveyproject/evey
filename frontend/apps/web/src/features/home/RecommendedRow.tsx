import { useFeedServiceRecommended } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { FeedItemCard } from "./FeedItemCard";

export function RecommendedRow() {
  const { data, isLoading } = useFeedServiceRecommended();
  const items = data?.items ?? [];

  if (!isLoading && items.length === 0) return null;

  return (
    <FeedSection title="Recommended for You" isLoading={isLoading}>
      {items.map((item) => (
        <FeedItemCard key={item.mediaItemId} item={item} />
      ))}
    </FeedSection>
  );
}
