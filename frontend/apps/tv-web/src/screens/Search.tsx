import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  getBaseURL,
  useLibraryServiceListLibraries,
  useLibraryServiceSearchMedia,
  useMusicServiceSearchMusic,
  type V1MediaItem,
  type V1Track,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { coverArtUri, selectSearchResultsView, useDebounce } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button, Card, TextField, Spinner } from "../components/ui";
import { navigate, replace } from "../app/router";
import { posterSrc } from "../lib/media";
import { useMusicQueueStore, trackToQueueTrack } from "../music/queue";

type Scope = "media" | "music";

function itemTitle(item: V1MediaItem): string {
  if (item.type === "MEDIA_ITEM_TYPE_EPISODE" && item.seriesTitle) return `${item.seriesTitle} - ${item.title ?? ""}`;
  const year = item.year ? ` (${item.year})` : "";
  return `${item.title ?? ""}${year}`;
}

function trackArt(url: string | undefined): string | undefined {
  if (!url) return undefined;
  const token = useAuthStore.getState().accessToken;
  const withToken = token ? `${url}${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : url;
  return coverArtUri(getBaseURL(), withToken, 400);
}

// Search, the way the Android TV app does it: the field takes the remote's
// keyboard, results follow the typing after a short pause, and the focus
// stays on the field while you type - the first result claiming it after
// every letter is what closed the keyboard on the Android app until
// 2026-09-22. The query rides the url so back returns to the same results.
// Two scopes as on the Android TV: films and series, or the music library's
// tracks (a track starts the queue with itself alone).
export function SearchScreen({ initialQuery }: { initialQuery?: string }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState(initialQuery ?? "");
  const [scope, setScope] = useState<Scope>("media");
  const debounced = useDebounce(query.trim(), 300);
  const enabled = debounced.length > 1;
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const { data: libs } = useLibraryServiceListLibraries();
  const hasMusic = (libs?.libraries ?? []).some((l) => l.type === "LIBRARY_TYPE_MUSIC");
  const effectiveScope: Scope = hasMusic ? scope : "media";

  const media = useLibraryServiceSearchMedia(
    { query: debounced, limit: 40 },
    { query: { enabled: enabled && effectiveScope === "media" } },
  );
  const music = useMusicServiceSearchMusic(
    { query: debounced, limit: 40 },
    { query: { enabled: enabled && effectiveScope === "music" } },
  );
  const items = (media.data?.items ?? []).filter((it) => it.id);
  const tracks: V1Track[] = (music.data?.tracks ?? []).filter((tr) => tr.id);
  const active = effectiveScope === "media" ? media : music;
  // The same decision the Android television makes, from @evey/client.
  const view = selectSearchResultsView({
    searchKind: effectiveScope,
    isError: active.isError,
    activeLoading: active.isLoading,
    hasQuery: enabled,
    musicItemsCount: tracks.length,
    mediaItemsCount: items.length,
  });

  useEffect(() => {
    if (debounced !== (initialQuery ?? "")) replace({ name: "search", query: debounced || undefined });
  }, [debounced, initialQuery]);

  return (
    <div className="screen">
      <FocusRoot autoFocus className="search-head">
        <TextField value={query} onChange={setQuery} placeholder={t("media.search.placeholder", "Search movies, series…")} autoFocus />
        {hasMusic ? (
          <div className="actions">
            <Button variant={effectiveScope === "media" ? "primary" : "default"} onPress={() => setScope("media")}>{t("media.search.sectionMedia", "Media")}</Button>
            <Button variant={effectiveScope === "music" ? "primary" : "default"} onPress={() => setScope("music")}>{t("media.search.sectionMusic", "Music")}</Button>
          </div>
        ) : null}
      </FocusRoot>
      {view === "prompt" ? (
        <p className="muted">{t("media.search.placeholder", "Search movies, series…")}</p>
      ) : view === "loading" ? (
        <Spinner />
      ) : view === "error" ? (
        <p className="error">{t("media.search.searchFailed", "Search failed")}</p>
      ) : view === "mediaEmpty" || view === "musicEmpty" ? (
        <p className="muted">{t("media.search.noResultsFor", { query: debounced, defaultValue: `No results for "${debounced}"` })}</p>
      ) : view === "mediaResults" ? (
        <FocusRoot>
          <div className="grid">
            {items.map((item) => (
              <Card key={item.id} focusKey={`search1:${item.id}`} title={itemTitle(item)} imageUrl={posterSrc(item.id, "poster", 400)} onPress={() => navigate({ name: "media", mediaId: item.id! })} />
            ))}
          </div>
        </FocusRoot>
      ) : (
        <FocusRoot>
          <div className="grid">
            {tracks.map((tr) => (
              <Card
                key={tr.id}
                focusKey={`search2:${tr.id}`}
                title={`${tr.title ?? ""}${tr.artist ? ` · ${tr.artist}` : ""}`}
                imageUrl={trackArt(tr.coverArtUrl)}
                onPress={() => setQueue([trackToQueueTrack(tr)], 0)}
              />
            ))}
          </div>
        </FocusRoot>
      )}
    </div>
  );
}
