import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Folder } from "lucide-react";
import { useCollectionServiceListCollectionsForMediaItem } from "@evey/api";

/**
 * The sagas a title belongs to, on the title's own page.
 *
 * Sagas were only reachable from the Collections screen, so someone looking at
 * Alien had no way to ask "what else of this do I have" - the one place they
 * actually wonder. Renders nothing when the title belongs to nothing, which is
 * the common case.
 */
export function SagaSection({ mediaItemId }: { mediaItemId: string }) {
  const { t } = useTranslation();
  const { data } = useCollectionServiceListCollectionsForMediaItem(mediaItemId, {
    query: { enabled: !!mediaItemId, staleTime: 5 * 60_000 },
  });
  const collections = data?.collections ?? [];
  if (collections.length === 0) return null;

  return (
    <section className="mt-8" data-testid="media-sagas">
      <h2 className="text-lg font-semibold text-[var(--text)] mb-3">
        {t("media.collections.partOf")}
      </h2>
      <div className="flex flex-wrap gap-2">
        {collections.map((c) => (
          <Link
            key={c.id}
            to="/collections/$collectionId"
            params={{ collectionId: c.id ?? "" }}
            data-testid={`media-saga-${c.id}`}
            className="inline-flex items-center gap-2 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface-raised)] transition-colors-fast"
          >
            <Folder size={16} className="text-[var(--text-muted)]" />
            <span>{c.name}</span>
            {(c.itemCount ?? 0) > 0 && (
              <span className="text-[var(--text-muted)]">
                {t("media.person.titleCount", { count: c.itemCount })}
              </span>
            )}
          </Link>
        ))}
      </div>
    </section>
  );
}
