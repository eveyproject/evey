import { jsx as _jsx } from "react/jsx-runtime";
import { useFeedServiceRecommended } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { FeedItemCard } from "./FeedItemCard";
export function RecommendedRow() {
    const { data, isLoading } = useFeedServiceRecommended();
    const items = data?.items ?? [];
    if (!isLoading && items.length === 0)
        return null;
    return (_jsx(FeedSection, { title: "Recommended for You", isLoading: isLoading, children: items.map((item) => (_jsx(FeedItemCard, { item: item }, item.mediaItemId))) }));
}
