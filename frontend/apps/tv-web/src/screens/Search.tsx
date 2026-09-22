import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, type V1MediaItem } from "@evey/api";
import { useDebounce } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Card, TextField, Spinner } from "../components/ui";
import { navigate, replace } from "../app/router";
import { posterSrc } from "../lib/media";

function itemTitle(item: V1MediaItem): string {
  if (item.type === "MEDIA_ITEM_TYPE_EPISODE" && item.seriesTitle) return `${item.seriesTitle} - ${item.title ?? ""}`;
  const year = item.year ? ` (${item.year})` : "";
  return `${item.title ?? ""}${year}`;
}

// Search, the way the Android TV app does it: the field takes the remote's
// keyboard, results follow the typing after a short pause, and the focus
// stays on the field while you type - the first result claiming it after
// every letter is what closed the keyboard on the Android app until
// 2026-09-22. The query rides the url so back returns to the same results.
export function SearchScreen({ initialQuery }: { initialQuery?: string }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState(initialQuery ?? "");
  const debounced = useDebounce(query.trim(), 300);
  const enabled = debounced.length > 1;
  const { data, isLoading, isError } = useLibraryServiceSearchMedia(
    { query: debounced, limit: 40 },
    { query: { enabled } },
  );
  const items = (data?.items ?? []).filter((it) => it.id);

  useEffect(() => {
    if (debounced !== (initialQuery ?? "")) replace({ name: "search", query: debounced || undefined });
  }, [debounced, initialQuery]);

  return (
    <div className="screen">
      <FocusRoot autoFocus className="search-head">
        <TextField value={query} onChange={setQuery} placeholder={t("media.search.placeholder", "Search movies, series…")} autoFocus />
      </FocusRoot>
      {!enabled ? (
        <p className="muted">{t("media.search.placeholder", "Search movies, series…")}</p>
      ) : isLoading ? (
        <Spinner />
      ) : isError ? (
        <p className="error">{t("media.search.searchFailed", "Search failed")}</p>
      ) : items.length === 0 ? (
        <p className="muted">{t("media.search.noResultsFor", { query: debounced, defaultValue: `No results for "${debounced}"` })}</p>
      ) : (
        <FocusRoot>
          <div className="grid">
            {items.map((item) => (
              <Card key={item.id} title={itemTitle(item)} imageUrl={posterSrc(item.id, "poster", 400)} onPress={() => navigate({ name: "media", mediaId: item.id! })} />
            ))}
          </div>
        </FocusRoot>
      )}
    </div>
  );
}
