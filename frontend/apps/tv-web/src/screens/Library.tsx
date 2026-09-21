import { useTranslation } from "react-i18next";
import { useLibraryServiceListMediaItems, type V1MediaItem } from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Card, Spinner } from "../components/ui";
import { navigate } from "../app/router";
import { posterSrc } from "../lib/media";

export function LibraryScreen({ libraryId }: { libraryId: string }) {
  const { t } = useTranslation();
  // One page of the library grid, largest the gateway allows in one call; a
  // paged scroll comes with the next milestone.
  const { data, isLoading, isError } = useLibraryServiceListMediaItems({ libraryId, "pagination.pageSize": 200 });
  const items = (data?.items ?? []).filter((it): it is V1MediaItem & { id: string } => !!it.id);

  return (
    <FocusRoot autoFocus className="screen">
      <h1 className="title">{t("library.title", "Library")}</h1>
      {isLoading ? <Spinner /> : null}
      {isError ? <p className="error">{t("common.error", "Something went wrong.")}</p> : null}
      <div className="grid">
        {items.map((item, i) => (
          <Card key={item.id} title={item.title ?? ""} imageUrl={posterSrc(item.id)} autoFocus={i === 0} onPress={() => navigate({ name: "media", mediaId: item.id })} />
        ))}
      </div>
    </FocusRoot>
  );
}
