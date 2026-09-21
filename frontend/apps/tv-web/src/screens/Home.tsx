import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetNextUp,
  usePlaybackServiceGetResumeItems,
  type V1Library,
  type V1MediaItem,
} from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Button, Card } from "../components/ui";
import { navigate } from "../app/router";
import { posterSrc, isPlayable } from "../lib/media";

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
            autoFocus={focusFirst && i === 0}
            onPress={() => openItem(item)}
          />
        ))}
      </div>
    </FocusRoot>
  );
}

function LibraryRow({ library }: { library: V1Library }) {
  const { data } = useLibraryServiceListMediaItems(
    { libraryId: library.id, "pagination.pageSize": 20 },
    { query: { enabled: !!library.id } },
  );
  const items = (data?.items ?? []).filter((it) => it.id);
  return <Row title={library.name ?? ""} items={items} />;
}

export function HomeScreen() {
  const { t } = useTranslation();
  const resume = usePlaybackServiceGetResumeItems({ limit: 20 });
  const nextUp = usePlaybackServiceGetNextUp({ limit: 20 });
  const libraries = useLibraryServiceListLibraries();

  const resumeItems = (resume.data?.items ?? []).filter(isPlayable);
  const nextItems = (nextUp.data?.items ?? []).filter(isPlayable);
  const libs = (libraries.data?.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MOVIES" || l.type === "LIBRARY_TYPE_SHOWS");
  const nothingYet = resumeItems.length === 0 && nextItems.length === 0;

  return (
    <FocusRoot autoFocus className="screen">
      <FocusRoot>
        <div className="actions" style={{ marginTop: 0 }}>
          <span className="title" style={{ marginRight: "auto" }}>Evey</span>
          {libs.map((l) => (
            <Button key={l.id} onPress={() => l.id && navigate({ name: "library", libraryId: l.id })}>{l.name}</Button>
          ))}
          <Button onPress={() => navigate({ name: "settings" })}>{t("settings.title", "Settings")}</Button>
        </div>
      </FocusRoot>
      <Row title={t("home.continueWatching", "Continue watching")} items={resumeItems} wide focusFirst />
      <Row title={t("home.nextUp", "Next up")} items={nextItems} wide focusFirst={resumeItems.length === 0} />
      {libs.map((l) => (
        <LibraryRow key={l.id} library={l} />
      ))}
      {nothingYet && libs.length === 0 && !libraries.isLoading ? (
        <p className="subtitle">{t("home.empty", "Nothing to show yet.")}</p>
      ) : null}
    </FocusRoot>
  );
}
