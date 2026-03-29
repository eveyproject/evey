import { memo } from "react";
import { useTranslation } from "react-i18next";
import { useFeedServiceRecentlyAdded } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { MediaCard } from "../library/MediaCard";

export const RecentlyAddedRow = memo(function RecentlyAddedRow() {
  const { t } = useTranslation();
  const { data, isLoading } = useFeedServiceRecentlyAdded();
  const items = data?.items ?? [];

  if (!isLoading && items.length === 0) return null;

  return (
    <FeedSection title={t("media.home.recentlyAdded")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showYear={false}
          showType
          lite
        />
      ))}
    </FeedSection>
  );
});
