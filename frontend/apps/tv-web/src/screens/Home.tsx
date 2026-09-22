import { useMemo, useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  eveyFetch,
  getLibraryServiceListMediaItemsQueryOptions,
  getMusicServiceGetAlbumsQueryOptions,
  useFeedServiceGetFeedConfig,
  useFeedServiceRecommended,
  useLibraryServiceListLibraries,
  usePlaybackServiceGetNextUp,
  usePlaybackServiceGetResumeItems,
  type V1MediaItem,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import {
  HOME_LIBRARY_ROW_PAGE_SIZE,
  buildHomeSections,
  resolveMaxItems,
  useCollections,
  useWatchProgressMaps,
  useMostPopular,
  type HomeSection,
} from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button, Card, Spinner } from "../components/ui";
import { navigate } from "../app/router";
import { posterSrc, isPlayable } from "../lib/media";
import { FeedCustomizePanel } from "../components/FeedCustomizePanel";
import { PluginSlot } from "../plugins/PluginSlot";

// The labels the other clients use for the transverse rows.
const FEED_LABELS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  continue_watching: "common.feeds.labels.resume_playback",
  next_episode: "media.home.upNext",
  collections: "media.home.collections",
  recommended: "media.home.recommendedForYou",
  most_popular: "media.home.mostPopular",
};

function itemTitle(item: V1MediaItem): string {
  if (item.type === "MEDIA_ITEM_TYPE_EPISODE" && item.seriesTitle) {
    const n = item.indexNumber ? ` ${item.indexNumber}` : "";
    return `${item.seriesTitle}${n} - ${item.title ?? ""}`;
  }
  return item.title ?? "";
}

function openItem(item: V1MediaItem) {
  if (!item.id) return;
  navigate({ name: "media", mediaId: item.id });
}

function Row({ title, items, wide, focusFirst }: { title: string; items: V1MediaItem[]; wide?: boolean; focusFirst?: boolean }) {
  const { watchedMap, percentMap } = useWatchProgressMaps();
  if (items.length === 0) return null;
  return (
    <FocusRoot>
      <h2 className="row-title">{title}</h2>
      <div className="row">
        {items.map((item, i) => (
          <Card
            key={item.id}
            title={itemTitle(item)}
            imageUrl={posterSrc(item.id, wide ? "backdrop" : "poster", wide ? 640 : 400)}
            wide={wide}
            watched={!!item.id && watchedMap.get(item.id) === true}
            percent={item.id ? percentMap.get(item.id) : undefined}
            autoFocus={focusFirst && i === 0}
            onPress={() => openItem(item)}
          />
        ))}
      </div>
    </FocusRoot>
  );
}

/**
 * The home, on the row plan the server holds: same feed types, same order and
 * same per-row item counts as the web, the phone and the Android television.
 * An admin who reorders the home reorders it here too, and the viewer can
 * reorder their own with the Customise panel.
 */
export function HomeScreen() {
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const [customizeOpen, setCustomizeOpen] = useState(false);

  const { data: feedConfigData } = useFeedServiceGetFeedConfig({ profileId });
  const libraries = useLibraryServiceListLibraries();
  const sections = useMemo(
    () => buildHomeSections(feedConfigData?.configs ?? [], libraries.data?.libraries ?? []),
    [feedConfigData, libraries.data],
  );

  const maxItemsByType = useMemo(() => {
    const map: Record<string, number> = {};
    for (const s of sections) {
      if (s.kind === "feed" && s.maxItems && s.maxItems > 0) map[s.feedType] = s.maxItems;
    }
    return map;
  }, [sections]);

  const resume = usePlaybackServiceGetResumeItems({ limit: resolveMaxItems(maxItemsByType.resume_playback, 20) });
  const nextUp = usePlaybackServiceGetNextUp({ limit: resolveMaxItems(maxItemsByType.next_episode, 20) });
  const recommended = useFeedServiceRecommended({ limit: resolveMaxItems(maxItemsByType.recommended, 20) });
  const popular = useMostPopular(resolveMaxItems(maxItemsByType.most_popular, 20));
  const collections = useCollections();

  // Every library row is fetched here rather than inside its component, so the
  // screen knows which rows have items: that is what lets the remote's initial
  // focus land on the first row that is actually there.
  const librarySections = useMemo(() => sections.filter((s): s is Extract<HomeSection, { kind: "library" }> => s.kind === "library"), [sections]);
  const libraryRows = useQueries({
    queries: librarySections.map((row) => {
      const limit = resolveMaxItems(row.maxItems, HOME_LIBRARY_ROW_PAGE_SIZE);
      if (row.libraryType === "LIBRARY_TYPE_MUSIC") return getMusicServiceGetAlbumsQueryOptions({ libraryId: row.libraryId });
      return getLibraryServiceListMediaItemsQueryOptions({
        libraryId: row.libraryId,
        ...(row.libraryType === "LIBRARY_TYPE_SHOWS" ? { type: "MEDIA_ITEM_TYPE_SERIES" as never } : {}),
        ...(row.libraryType === "LIBRARY_TYPE_MOVIES" ? { type: "MEDIA_ITEM_TYPE_MOVIE" as never } : {}),
        sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
        sortOrder: "SORT_ORDER_DESC",
        "pagination.pageSize": limit,
      });
    }),
  });

  // Names for the Customise panel: a library row would otherwise read as a raw
  // identifier, which is unusable across a room.
  const libraryNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const lib of libraries.data?.libraries ?? []) if (lib.id && lib.name) map[lib.id] = lib.name;
    return map;
  }, [libraries.data]);
  // Only a plugin shipping a home.section module may be offered as a row.
  const { data: pluginsData } = useQuery<{ plugins?: { id?: string; name?: string; frontend?: { slot?: string }[] }[] }>({
    queryKey: ["/api/v1/plugins"],
    queryFn: () => eveyFetch({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
  });
  const pluginNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const p of pluginsData?.plugins ?? []) {
      if (!p.id || !p.name) continue;
      if (!(p.frontend ?? []).some((m) => m.slot === "home.section")) continue;
      map[p.id] = p.name;
    }
    return map;
  }, [pluginsData]);

  // The plugins the feed config already places are rendered at their row; the
  // rest appear once, above, so a fresh install is visible immediately.
  const registeredPlugins = useMemo(
    () => new Set(sections.filter((s) => s.kind === "plugin").map((s) => (s as Extract<HomeSection, { kind: "plugin" }>).pluginId)),
    [sections],
  );

  const itemsFor = (section: HomeSection): V1MediaItem[] => {
    if (section.kind !== "feed") return [];
    switch (section.feedType) {
      case "resume_playback":
      case "continue_watching":
        return (resume.data?.items ?? []).filter(isPlayable);
      case "next_episode":
        return (nextUp.data?.items ?? []).filter(isPlayable);
      // Two different payload shapes, projected onto a media item so one card
      // serves every row - the same projection the television and the phone do.
      case "recommended":
        return (recommended.data?.items ?? [])
          .filter((i) => !!i.mediaItemId)
          .map((i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem);
      case "most_popular":
        return (popular.data ?? []).map((i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem);
      default:
        return [];
    }
  };

  const isLoading = resume.isLoading || nextUp.isLoading || libraries.isLoading;
  // The first row that has something claims the focus, so the remote always
  // starts on content rather than on an empty shelf.
  let focusClaimed = false;
  const claimFocus = (has: boolean) => {
    if (!has || focusClaimed) return false;
    focusClaimed = true;
    return true;
  };

  const rows = sections.map((section) => {
    if (section.kind === "library") {
      const index = librarySections.indexOf(section as Extract<HomeSection, { kind: "library" }>);
      const query = libraryRows[index];
      const data = query?.data as { items?: V1MediaItem[]; albums?: { id?: string; title?: string; artist?: string }[] } | undefined;
      if (section.libraryType === "LIBRARY_TYPE_MUSIC") {
        const albums = (data?.albums ?? []).filter((a) => a.id);
        if (albums.length === 0) return null;
        return (
          <FocusRoot key={section.key}>
            <h2 className="row-title">{section.name}</h2>
            <div className="row">
              {albums.map((a, i) => (
                <Card
                  key={a.id}
                  title={`${a.title ?? ""}${a.artist ? ` · ${a.artist}` : ""}`}
                  autoFocus={i === 0 && claimFocus(true)}
                  onPress={() => navigate({ name: "album", albumId: a.id!, libraryId: section.libraryId })}
                />
              ))}
            </div>
          </FocusRoot>
        );
      }
      const items = (data?.items ?? []).filter((it) => it.id);
      if (items.length === 0) return null;
      return <Row key={section.key} title={section.name} items={items} focusFirst={claimFocus(true)} />;
    }
    if (section.kind === "plugin") {
      return (
        <div className="plugin-row" key={section.key}>
          <PluginSlot name="home.section" pluginId={section.pluginId} />
        </div>
      );
    }
    if (section.feedType === "collections") {
      const list = (collections.data?.collections ?? []).filter((c) => c.id);
      if (list.length === 0) return null;
      return (
        <FocusRoot key={section.key}>
          <h2 className="row-title">{t(FEED_LABELS.collections, "Collections")}</h2>
          <div className="row">
            {list.map((c, i) => (
              <Card key={c.id} title={c.name} imageUrl={c.posterUrl} autoFocus={i === 0 && claimFocus(true)} onPress={() => navigate({ name: "collection", collectionId: c.id })} />
            ))}
          </div>
        </FocusRoot>
      );
    }
    const items = itemsFor(section);
    // A row with nothing in it returns NOTHING, so "the home is empty" can be
    // read off this list. Returning a Row that draws nothing left the page
    // unable to ever say it had no content.
    if (items.length === 0) return null;
    const wide = section.feedType === "resume_playback" || section.feedType === "continue_watching" || section.feedType === "next_episode";
    return (
      <Row
        key={section.key}
        title={t(FEED_LABELS[section.feedType] ?? section.feedType, section.feedType)}
        items={items}
        wide={wide}
        focusFirst={claimFocus(true)}
      />
    );
  });

  const nothing = !isLoading && rows.every((r) => r === null);
  const libs = (libraries.data?.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MOVIES" || l.type === "LIBRARY_TYPE_SHOWS");
  const hasMusic = (libraries.data?.libraries ?? []).some((l) => l.type === "LIBRARY_TYPE_MUSIC");

  return (
    <FocusRoot autoFocus className="screen">
      <FocusRoot>
        <div className="actions" style={{ marginTop: 0 }}>
          <span className="title" style={{ marginRight: "auto" }}>Evey</span>
          {libs.map((l) => (
            <Button key={l.id} onPress={() => l.id && navigate({ name: "library", libraryId: l.id })}>{l.name}</Button>
          ))}
          {hasMusic ? <Button onPress={() => navigate({ name: "music" })}>{t("common.nav.music", "Music")}</Button> : null}
          <Button onPress={() => navigate({ name: "search" })}>{t("common.nav.search", "Search")}</Button>
          <Button onPress={() => setCustomizeOpen(true)}>{t("media.home.customize", "Customise")}</Button>
          <Button onPress={() => navigate({ name: "settings" })}>{t("common.nav.settings", "Settings")}</Button>
        </div>
      </FocusRoot>
      {isLoading ? <Spinner /> : null}
      <div className="plugin-row">
        <PluginSlot name="home.section" excludeRegistered={registeredPlugins} />
      </div>
      {rows}
      {nothing ? <p className="subtitle">{t("media.home.emptyLibrary", "Nothing to show yet.")}</p> : null}
      {customizeOpen ? (
        <FeedCustomizePanel
          configs={feedConfigData?.configs ?? []}
          profileId={profileId}
          libraryNames={libraryNames}
          pluginNames={pluginNames}
          onClose={() => setCustomizeOpen(false)}
        />
      ) : null}
    </FocusRoot>
  );
}
