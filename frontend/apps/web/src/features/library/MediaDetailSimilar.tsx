import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import { useFeedServiceSimilarItems } from "@evey/api";
import { feedItemsToCards } from "@evey/client";
import { MediaCardWithModals as MediaCard } from "./MediaCardWithModals";

export function SimilarSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
  // The server's list or nothing: a same-genre fallback filled the row with
  // titles sharing one catch-all genre, which is not "similar" (2026-10-01).
  const cards = feedItemsToCards(similarData?.items);

  if (!cards.length) return null;

  return (
    <div className="mt-10">
      <h2 className="text-lg font-semibold mb-4 text-[var(--text)]">{t("media.detail.similarTitles")}</h2>
      <div className="flex gap-4 overflow-x-auto pb-2 scrollbar-thin">
        {cards.map((c) => (
          <MediaCard key={c.id} item={c} lite />
        ))}
      </div>
    </div>
  );
}
