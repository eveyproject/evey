import { memo, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { usePlaybackServiceGetResumeItems } from "@evey/api";
import { FeedSection } from "./FeedSection";
import { MediaCardWithModals as MediaCard } from "../library/MediaCardWithModals";

export const ContinueWatchingRow = memo(function ContinueWatchingRow() {
  const { t } = useTranslation();
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
    <FeedSection title={t("media.home.continueWatching")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.id}
          item={item}
          showYear={false}
          showProgress
          progressPercent={progressMap.get(item.id ?? "")}
          lite
        />
      ))}
    </FeedSection>
  );
});
