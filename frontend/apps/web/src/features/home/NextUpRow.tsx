import { memo, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { usePlaybackServiceGetNextUp } from "@evey/api";
import { MediaSlider } from "./MediaSlider";
import { MediaCardWithModals as MediaCard } from "../library/MediaCardWithModals";
import { usePlaybackDifficulties } from "@/hooks/usePlaybackDifficulties";

export const NextUpRow = memo(function NextUpRow({ maxItems }: { maxItems?: number } = {}) {
  const { t } = useTranslation();
  const { data, isLoading } = usePlaybackServiceGetNextUp({ limit: maxItems ?? 20 });
  const items = data?.items ?? [];

  // One batched call for the whole row, so a card can warn that a title is
  // heavy to stream before it is started. Never one call per card.
  const ids = useMemo(
    () => items.map((i) => i.id).filter((id): id is string => !!id),
    [items],
  );
  const difficulties = usePlaybackDifficulties(ids);

  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("media.home.upNext")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard key={item.id} item={item} difficulty={difficulties.get(item.id ?? "")} showYear={false} lite />
      ))}
    </MediaSlider>
  );
});
