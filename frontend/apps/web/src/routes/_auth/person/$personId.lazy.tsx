import { useMemo } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceListMediaItems } from "@evey/api";
import type { V1MediaItem, V1MediaPerson } from "@evey/api/src/generated/library.schemas";
import { BackButton } from "@/components/ui/BackButton";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { MediaCard } from "@/features/library/MediaCard";

export const Route = createLazyFileRoute("/_auth/person/$personId")({
  component: PersonPage,
});

const ROLE_KEYS: Record<string, string> = {
  PERSON_TYPE_ACTOR: "media.person.actor",
  PERSON_TYPE_DIRECTOR: "media.person.director",
  PERSON_TYPE_WRITER: "media.person.writer",
  PERSON_TYPE_PRODUCER: "media.person.producer",
  PERSON_TYPE_COMPOSER: "media.person.composer",
  PERSON_TYPE_GUEST_STAR: "media.person.guestStar",
};

interface FilmographyEntry {
  item: V1MediaItem;
  person: V1MediaPerson;
}

function usePersonFilmography(personId: string) {
  const { data: moviesData, isLoading: loadingMovies } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_MOVIE" as never,
    "pagination.pageSize": 200,
  });
  const { data: seriesData, isLoading: loadingSeries } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_SERIES" as never,
    "pagination.pageSize": 200,
  });

  return useMemo(() => {
    const allItems = [...(moviesData?.items ?? []), ...(seriesData?.items ?? [])];
    const entries: FilmographyEntry[] = [];
    let personInfo: V1MediaPerson | null = null;

    for (const item of allItems) {
      const match = item.people?.find((p) => p.id === personId);
      if (match) {
        entries.push({ item, person: match });
        if (!personInfo || (match.thumbUrl && !personInfo.thumbUrl)) {
          personInfo = match;
        }
      }
    }

    const roles = new Set<string>();
    for (const e of entries) {
      if (e.person.type) roles.add(e.person.type);
    }

    entries.sort((a, b) => (b.item.year ?? 0) - (a.item.year ?? 0));

    return { entries, personInfo, roles: Array.from(roles), isLoading: loadingMovies || loadingSeries };
  }, [moviesData, seriesData, personId, loadingMovies, loadingSeries]);
}

function PersonPage() {
  const { t } = useTranslation();
  const { personId } = Route.useParams();
  const { entries, personInfo, roles, isLoading } = usePersonFilmography(personId);

  if (!personId) {
    return (
      <div className="text-center py-16" style={{ color: "var(--text-muted)" }}>
        {t("media.person.notFound")}
      </div>
    );
  }

  const name = personInfo?.name ?? t("media.person.unknown");
  const initial = name[0]?.toUpperCase() ?? "?";
  const thumbUrl = personInfo?.thumbUrl;

  const byRole = new Map<string, FilmographyEntry[]>();
  for (const entry of entries) {
    const role = entry.person.type ?? "PERSON_TYPE_UNSPECIFIED";
    if (!byRole.has(role)) byRole.set(role, []);
    byRole.get(role)!.push(entry);
  }

  return (
    <div className="max-w-5xl mx-auto">
      <BackButton fallback="/library" className="mb-6" />

      <div className="flex items-start gap-6 mb-10">
        <div
          className="w-32 h-32 rounded-full overflow-hidden flex-shrink-0 flex items-center justify-center"
          style={{ backgroundColor: "var(--surface-raised)" }}
        >
          {thumbUrl ? (
            <img
              src={thumbUrl}
              alt={name}
              className="w-full h-full object-cover"
              loading="lazy"
              onError={(e) => {
                const t = e.currentTarget;
                t.style.display = "none";
                const fb = t.nextElementSibling as HTMLElement | null;
                if (fb) fb.style.display = "flex";
              }}
            />
          ) : null}
          <span
            className="text-4xl font-bold items-center justify-center"
            style={{ color: "var(--text-muted)", display: thumbUrl ? "none" : "flex" }}
          >
            {initial}
          </span>
        </div>
        <div className="flex-1 min-w-0 pt-2">
          <h1 className="text-3xl font-bold mb-2" style={{ color: "var(--text)" }}>{name}</h1>
          <div className="flex flex-wrap gap-2 mb-3">
            {roles.map((r) => (
              <Badge key={r} variant="info">{t(ROLE_KEYS[r] ?? "media.person.other")}</Badge>
            ))}
          </div>
          {!isLoading && (
            <p className="text-sm" style={{ color: "var(--text-muted)" }}>
              {entries.length === 1
                ? t("media.person.titleCount", { count: entries.length })
                : t("media.person.titleCount_plural", { count: entries.length })}
            </p>
          )}
        </div>
      </div>

      {isLoading && (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      )}

      {!isLoading && entries.length === 0 && (
        <div className="text-center py-16" style={{ color: "var(--text-muted)" }}>
          {t("media.person.noFilmography")}
        </div>
      )}

      {Array.from(byRole.entries()).map(([role, items]) => (
        <section key={role} className="mb-10">
          <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
            {t(ROLE_KEYS[role] ?? "media.person.other")}
            <span className="text-sm font-normal ml-2" style={{ color: "var(--text-muted)" }}>
              ({items.length})
            </span>
          </h2>
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
            {items.map(({ item, person }) => (
              <div key={item.id} className="relative">
                <MediaCard item={item} lite />
                {person.role && (
                  <p className="text-xs mt-1 truncate px-0.5" style={{ color: "var(--text-muted)" }}>
                    {t("media.person.asRole", { role: person.role })}
                  </p>
                )}
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
