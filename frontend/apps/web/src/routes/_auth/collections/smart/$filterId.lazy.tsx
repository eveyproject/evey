import { useState } from "react";
import { Folder } from "lucide-react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useCollection, useDeleteCollection } from "@/features/collection/api";
import { Spinner } from "@/components/ui/Spinner";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { BackButton } from "@/components/ui/BackButton";
import { Pagination } from "@/components/ui/Pagination";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { toast, useAuthStore } from "@evey/store";

export const Route = createLazyFileRoute("/_auth/collections/smart/$filterId")({
  component: SmartFilterPage,
});

const OP_LABELS: Record<string, string> = {
  eq: "=",
  gte: ">=",
  lte: "<=",
  gt: ">",
  lt: "<",
};

function SmartFilterPage() {
  const { t } = useTranslation();
  const { filterId } = Route.useParams();
  const navigate = Route.useNavigate();
  // A smart collection is rule-based, so it routinely matches more titles than
  // one page holds. The header already showed the server's real total while the
  // body only ever fetched the default page - the same "announce 7199, render
  // 100" shape the photo grid shipped with. Paged like its sibling
  // collections/$collectionId, which already had this.
  const [page, setPage] = useState(1);
  const pageSize = 50;
  const { data, isLoading, isError, error, refetch } = useCollection(filterId, pageSize, (page - 1) * pageSize);
  const deleteMut = useDeleteCollection();
  const isAdmin = useAuthStore((s) => s.isAdmin);

  const col = data?.collection;
  const items = data?.items ?? [];
  const rules = col?.rules ?? [];
  const total = data?.total ?? items.length;
  const totalPages = Math.ceil(total / pageSize);

  function handleDelete() {
    if (!confirm(t("media.collections.deleteSmartConfirm"))) return;
    deleteMut.mutate(filterId, {
      onSuccess: () => {
        toast.success(t("media.collections.smartDeleted"));
        navigate({ to: "/collections" });
      },
      onError: () => toast.error(t("media.collections.deleteFailed")),
    });
  }

  // A server that did not answer is not a collection that was deleted: this
  // screen said "collection not found" on any failure, so a network blip read
  // as "someone removed it".
  if (isError) {
    return (
      <div>
        <BackButton fallback="/collections" />
        <FetchErrorState error={error} onRetry={() => refetch()} />
      </div>
    );
  }

  if (!isLoading && !col) {
    return (
      <div>
        <BackButton fallback="/collections" />
        <EmptyState icon={<Folder size={48} strokeWidth={1.5} />} title={t("media.collections.notFound")} description={t("media.collections.notFoundDesc")} />
      </div>
    );
  }

  return (
    <div>
      <BackButton fallback="/collections" />

      <div className="mt-4 mb-6 flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1 flex-wrap">
            <h1 className="text-2xl font-bold text-[var(--text)]">
              {col?.name ?? "\u2026"}
            </h1>
            <span
              className="text-xs font-semibold px-2 py-0.5 rounded-full bg-[var(--accent)] text-[var(--filter-active-text)]"
            >
              {t("media.collections.smart")}
            </span>
          </div>
          {rules.length > 0 && (
            <div className="flex flex-wrap gap-1.5 mt-2">
              {rules.map((r, i) => (
                <Badge key={i} variant="default">
                  {r.field} {OP_LABELS[r.operator] ?? r.operator} {r.value}
                </Badge>
              ))}
            </div>
          )}
          {!isLoading && (
            <p className="text-sm mt-2 text-[var(--text-muted)]">
              {t("common.item", { count: total })}
            </p>
          )}
        </div>
        {isAdmin && (
          <div className="flex gap-2 flex-shrink-0">
            <Button variant="danger" onClick={handleDelete} type="button" loading={deleteMut.isPending} data-testid="smart-delete-btn">
              {t("common.delete")}
            </Button>
          </div>
        )}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner /></div>
      ) : items.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.search.noResults")}
            description={rules.length === 0 ? t("media.collections.noRules") : t("media.collections.noRuleMatch")}
          />
        </div>
      ) : (
        <div className="border border-[var(--border)] rounded-[var(--radius-md)]">
          <div className="divide-y divide-[var(--border)]">
            {items.map((item, i) => (
              <Link
                key={item.id}
                to="/media/$mediaId"
                params={{ mediaId: item.id }}
                className="flex items-center gap-3 px-4 py-3 transition-colors-fast hover:bg-[var(--surface-raised)] text-[var(--text)]"
              >
                <span className="text-sm font-mono w-6 flex-shrink-0 text-[var(--accent)]">
                  {i + 1}
                </span>
                <span className="text-sm flex-1 truncate">
                  {item.title ?? item.id}
                </span>
                {item.type && (
                  <span className="text-xs flex-shrink-0 text-[var(--text-muted)]">
                    {item.type.replace("MEDIA_ITEM_TYPE_", "").toLowerCase()}
                  </span>
                )}
                {!!item.year && (
                  <span className="text-xs flex-shrink-0 text-[var(--text-muted)]">{item.year}</span>
                )}
              </Link>
            ))}
          </div>
          <div className="px-4 py-2 text-xs text-[var(--text-muted)] border-t border-[var(--border)]">
            {t("common.item", { count: total })}
          </div>
        </div>
      )}
      {totalPages > 1 && (
        <div className="mt-6">
          <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
        </div>
      )}
    </div>
  );
}
