import { memo } from "react";
import { useTranslation } from "react-i18next";
import { usePlaybackServiceGetNextUp } from "@evey/api";
import { MediaSlider } from "./MediaSlider";
import { MediaCardWithModals as MediaCard } from "../library/MediaCardWithModals";

export const NextUpRow = memo(function NextUpRow({ maxItems }: { maxItems?: number } = {}) {
  const { t } = useTranslation();
  const { data, isLoading } = usePlaybackServiceGetNextUp({ limit: maxItems ?? 20 });
  const items = data?.items ?? [];


  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("media.home.upNext")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard key={item.id} item={item} showYear={false} lite />
      ))}
    </MediaSlider>
  );
});
