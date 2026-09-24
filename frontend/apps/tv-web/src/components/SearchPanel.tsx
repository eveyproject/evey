import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  displayTitle,
  getBaseURL,
  useLibraryServiceSearchMedia,
  useMusicServiceSearchMusic,
  type V1MediaItem,
  type V1Track,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { coverArtUri, mediaTypeLabelKey, selectSearchResultsView, useDebounce } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Pressable, Spinner, TextField } from "./ui";
import { Icon } from "./icons";
import { navigate } from "../app/router";
import { posterSrc } from "../lib/media";
import { useMusicQueueStore, trackToQueueTrack } from "../music/queue";

type Scope = "media" | "music";

function trackArt(url: string | undefined): string | undefined {
  if (!url) return undefined;
  const token = useAuthStore.getState().accessToken;
  const withToken = token ? `${url}${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : url;
  return coverArtUri(getBaseURL(), withToken, 144);
}

/** The Android television's search panel, for the header's overlay and the tab. The
 * focus stays on the field while typing: a result claiming it closed the keyboard. */
export function SearchPanel({ initialQuery, onNavigate, onQueryChange }: {
  initialQuery?: string;
  onNavigate?: () => void;
  onQueryChange?: (query: string) => void;
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState(initialQuery ?? "");
  const [scope, setScope] = useState<Scope>("media");
  const debounced = useDebounce(query.trim(), 300);
  const enabled = debounced.length > 1;
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const media = useLibraryServiceSearchMedia({ query: debounced, limit: 30 }, { query: { enabled: enabled && scope === "media" } });
  const music = useMusicServiceSearchMusic({ query: debounced, limit: 30 }, { query: { enabled: enabled && scope === "music" } });
  const items = (media.data?.items ?? []).filter((it) => it.id);
  const tracks: V1Track[] = (music.data?.tracks ?? []).filter((tr) => tr.id);
  const active = scope === "media" ? media : music;
  const view = selectSearchResultsView({
    searchKind: scope,
    isError: active.isError,
    activeLoading: active.isLoading,
    hasQuery: enabled,
    musicItemsCount: tracks.length,
    mediaItemsCount: items.length,
  });

  const change = (v: string) => {
    setQuery(v);
    onQueryChange?.(v);
  };

  const openMedia = (item: V1MediaItem) => {
    onNavigate?.();
    navigate({ name: "media", mediaId: item.id! });
  };

  return (
    <div className="search-panel">
      <FocusRoot className="search-head">
        <div className="search-row">
          <TextField value={query} onChange={change} placeholder={t("media.search.placeholder")} autoFocus />
        </div>
        <div className="scope-tabs">
          <Pressable className={`scope-tab ${scope === "media" ? "active" : ""}`} onPress={() => setScope("media")}>{t("media.search.sectionMedia")}</Pressable>
          <Pressable className={`scope-tab ${scope === "music" ? "active" : ""}`} onPress={() => setScope("music")}>{t("media.search.sectionMusic")}</Pressable>
        </div>
      </FocusRoot>
      <div className="search-results">
        {view === "error" ? (
          <FocusRoot className="center-fill">
            <p className="muted big">{t("common.loadError")}</p>
            <Pressable className="link-btn" onPress={() => void active.refetch()}>{t("common.retry")}</Pressable>
          </FocusRoot>
        ) : view === "loading" ? (
          <div className="center-fill"><Spinner /></div>
        ) : view === "prompt" ? (
          <div className="center-fill"><p className="muted big">{t("media.search.searchLibrary")}</p></div>
        ) : view === "mediaEmpty" || view === "musicEmpty" ? (
          <div className="center-fill"><p className="muted big">{t("media.search.noResults")}</p></div>
        ) : view === "mediaResults" ? (
          <FocusRoot className="result-list">
            {items.map((item) => {
              const typeKey = mediaTypeLabelKey(item.type);
              const genres = (item.genres ?? []).filter(Boolean).slice(0, 2);
              return (
                <Pressable key={item.id} focusKey={`search:${item.id}`} className="result-row tv-card" onPress={() => openMedia(item)} label={displayTitle(item, t)}>
                  <span className="result-thumb">
                    <img src={posterSrc(item.id, "poster", 160)} alt="" loading="lazy" />
                  </span>
                  <span className="result-info">
                    <span className="result-title">{displayTitle(item, t)}</span>
                    <span className="result-meta">
                      {item.year ? <span>{item.year}</span> : null}
                      {typeKey ? <span className="type-badge">{t(typeKey)}</span> : null}
                      {genres.length > 0 ? <span>{genres.join(", ")}</span> : null}
                    </span>
                  </span>
                </Pressable>
              );
            })}
          </FocusRoot>
        ) : (
          <FocusRoot className="result-list">
            {tracks.map((tr) => {
              const art = trackArt(tr.coverArtUrl);
              return (
                <Pressable key={tr.id} focusKey={`search:${tr.id}`} className="result-row tv-card" onPress={() => { onNavigate?.(); setQueue([trackToQueueTrack(tr)], 0); }} label={tr.title}>
                  <span className="result-thumb square">
                    {art ? <img src={art} alt="" loading="lazy" /> : <Icon name="musical-notes" size={24} />}
                  </span>
                  <span className="result-info">
                    <span className="result-title">{tr.title}</span>
                    <span className="result-meta">{[tr.artist, tr.album].filter(Boolean).join(" · ")}</span>
                  </span>
                  <span className="accent"><Icon name="play" size={24} /></span>
                </Pressable>
              );
            })}
          </FocusRoot>
        )}
      </div>
    </div>
  );
}
