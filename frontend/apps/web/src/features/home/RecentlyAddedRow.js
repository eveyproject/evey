import { jsx as _jsx } from "react/jsx-runtime";
import { useFeedServiceRecentlyAdded } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { FeedItemCard } from "./FeedItemCard";
export function RecentlyAddedRow() {
    const { data, isLoading } = useFeedServiceRecentlyAdded();
    const items = data?.items ?? [];
    if (!isLoading && items.length === 0)
        return null;
    return (_jsx(FeedSection, { title: "Recently Added", isLoading: isLoading, children: items.map((item) => (_jsx(FeedItemCard, { item: item }, item.mediaItemId))) }));
}
