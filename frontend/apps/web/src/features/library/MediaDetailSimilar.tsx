import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import { useFeedServiceSimilarItems, useLibraryServiceListMediaItems } from "@evey/api";
import { MediaCard } from "./MediaCard";

export function SimilarSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
  const firstGenre = item.genres?.[0];
  const hasSimilar = (similarData?.items?.length ?? 0) > 0;
  const { data: byGenreData } = useLibraryServiceListMediaItems(
    { genre: firstGenre, type: item.type as never },
    { query: { enabled: !hasSimilar && !!firstGenre } } as never
  );

  const fromFeed: Array<{ id: string; title?: string }> =
    (similarData?.items ?? [])
      .filter((fi) => !!fi.mediaItemId)
      .map((fi) => ({ id: fi.mediaItemId!, title: fi.title }));

  const fromGenre: Array<{ id: string; title?: string; year?: number; type?: string }> =
    (byGenreData?.items ?? []).filter((m) => m.id && m.id !== item.id).map((m) => ({
      id: m.id!, title: m.title, year: m.year, type: m.type,
    }));

  const cards = fromFeed.length ? fromFeed : fromGenre;
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
