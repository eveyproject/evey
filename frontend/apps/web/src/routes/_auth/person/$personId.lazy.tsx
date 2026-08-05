import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceListMediaItems, useLibraryServiceGetPerson } from "@evey/api";
import { BackButton } from "@/components/ui/BackButton";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { MediaCardWithModals as MediaCard } from "@/features/library/MediaCardWithModals";

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

function PersonPage() {
  const { t } = useTranslation();
  const { personId } = Route.useParams();

  const { data: person } = useLibraryServiceGetPerson(personId);
  const { data, isLoading } = useLibraryServiceListMediaItems({
    personId,
    "pagination.pageSize": 200,
  });
  const items = data?.items ?? [];

  const name = person?.name ?? t("media.person.unknown");
  const initial = name[0]?.toUpperCase() ?? "?";
  const thumbUrl = person?.thumbUrl;

  // Group items by role
  const byRole = new Map<string, typeof items>();
  for (const item of items) {
    const match = item.people?.find((p) => p.id === personId);
    const role = match?.type ?? "PERSON_TYPE_UNSPECIFIED";
    if (!byRole.has(role)) byRole.set(role, []);
    byRole.get(role)!.push(item);
  }
  const roles = Array.from(byRole.keys());

  return (
    <div className="max-w-5xl mx-auto">
      <BackButton fallback="/library" className="mb-6" />

      <div className="flex items-start gap-6 mb-10">
        <div
          className="w-32 h-32 rounded-full overflow-hidden flex-shrink-0 flex items-center justify-center bg-[var(--surface-raised)]"
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
            className="text-4xl font-bold items-center justify-center text-[var(--text-muted)]"
            style={{ display: thumbUrl ? "none" : "flex" }}
          >
            {initial}
          </span>
        </div>
        <div className="flex-1 min-w-0 pt-2">
          <h1 className="text-3xl font-bold mb-2 text-[var(--text)]">{name}</h1>
          <div className="flex flex-wrap gap-2 mb-3">
            {roles.map((r) => (
              <Badge key={r} variant="info">{t(ROLE_KEYS[r] ?? "media.person.other")}</Badge>
            ))}
          </div>
          {!isLoading && (
            <p className="text-sm text-[var(--text-muted)]">
              {/* i18next v4 picks titleCount_one / titleCount_other from {count};
                  the manual _plural suffix (v3 style) has no matching key and
                  rendered the raw key for any count != 1. */}
              {t("media.person.titleCount", { count: items.length })}
            </p>
          )}
          {(person?.birthday || person?.placeOfBirth || person?.knownFor) && (
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-[var(--text-muted)] mt-2">
              {/* knownFor is TMDB's known_for_department (Acting, Directing…),
                  fetched and stored since the people enrichment landed but never
                  rendered by any client. */}
              {person.knownFor && <span>{person.knownFor}</span>}
              {person.birthday && (
                <span>
                  {new Date(person.birthday).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}
                  {person.deathday
                    ? ` - ${new Date(person.deathday).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}`
                    : ` - ${Math.floor((Date.now() - new Date(person.birthday).getTime()) / 31557600000)} ${t("media.person.yearsOld")}`}
                </span>
              )}
              {person.placeOfBirth && <span>{person.placeOfBirth}</span>}
            </div>
          )}
        </div>
      </div>

      {person?.biography && (
        <p className="text-sm leading-relaxed text-[var(--text-secondary)] mb-8 max-w-3xl">
          {person.biography}
        </p>
      )}

      {isLoading && (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      )}

      {!isLoading && items.length === 0 && (
        <div className="text-center py-16 text-[var(--text-muted)]">
          {t("media.person.noFilmography")}
        </div>
      )}

      {Array.from(byRole.entries()).map(([role, roleItems]) => (
        <section key={role} className="mb-10">
          <h2 className="text-lg font-semibold mb-4 text-[var(--text)]">
            {t(ROLE_KEYS[role] ?? "media.person.other")}
            <span className="text-sm font-normal ml-2 text-[var(--text-muted)]">
              ({roleItems.length})
            </span>
          </h2>
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
            {roleItems.map((item) => {
              const person = item.people?.find((p) => p.id === personId);
              return (
                <div key={item.id} className="relative">
                  <MediaCard item={item} lite />
                  {person?.role && (
                    <p className="text-xs mt-1 truncate px-0.5 text-[var(--text-muted)]">
                      {t("media.person.asRole", { role: person.role })}
                    </p>
                  )}
                </div>
              );
            })}
          </div>
        </section>
      ))}
    </div>
  );
}
