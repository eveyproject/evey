import { useTranslation } from "react-i18next";
import { useFeedServiceRecommended } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { MediaCard } from "../library/MediaCard";

export function RecommendedRow() {
  const { t } = useTranslation();
  const { data, isLoading } = useFeedServiceRecommended();
  const items = data?.items ?? [];

  if (!isLoading && items.length === 0) return null;

  return (
    <FeedSection title={t("media.home.recommendedForYou")} isLoading={isLoading}>
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
}
