import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceListMediaItems } from "@evey/api";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
import { EmptyState } from "@/components/ui/EmptyState";
import { BackButton } from "@/components/ui/BackButton";
import type { SmartFilter, SmartRule } from "@/features/collection/SmartCollectionDialog";

export const Route = createLazyFileRoute("/_auth/collections/smart/$filterId")({
  component: SmartFilterPage,
});

function loadFilter(id: string): SmartFilter | null {
  try {
    const all = JSON.parse(localStorage.getItem("evey:smart-filters") ?? "[]") as SmartFilter[];
    return all.find((f) => f.id === id) ?? null;
  } catch {
    return null;
  }
}

function buildParams(rules: SmartRule[]) {
  const params: Record<string, string | number> = {};
  for (const r of rules) {
    if (r.field === "genre" && r.value) params.genre = r.value;
    if (r.field === "year" && r.value) {
      // server only supports exact year filter — use the value regardless of op
      params.year = Number(r.value);
    }
    if (r.field === "type" && r.value) params.type = r.value;
  }
  return params;
}

function SmartFilterPage() {
  const { filterId } = Route.useParams();
  const filter = loadFilter(filterId);

  const params = filter ? buildParams(filter.rules) : {};
  const hasParams = Object.keys(params).length > 0;

  const { data, isLoading } = useQuery({
    queryKey: ["smart-filter", filterId, params],
    queryFn: ({ signal }) =>
      libraryServiceListMediaItems({ ...params, "pagination.pageSize": 200 }, signal),
    enabled: !!filter && hasParams,
  });

  const items = data?.items ?? [];
  const total = data?.pagination?.totalItems ?? items.length;

  if (!filter) {
    return (
      <div>
        <BackButton fallback="/collections" />
        <EmptyState icon="library" title="Filter not found" description="This smart filter no longer exists." />
      </div>
    );
  }

  return (
    <div>
      <BackButton fallback="/collections" />

      <div className="mt-4 mb-6">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          {filter.name}
        </h1>
        {!isLoading && hasParams && (
          <p className="text-sm mt-1" style={{ color: "var(--text-muted)" }}>
            {total} {total === 1 ? "item" : "items"}
          </p>
        )}
      </div>

      {!hasParams ? (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <EmptyState
            icon="library"
            title="No rules defined"
            description="Edit this smart filter to add rules."
          />
        </div>
      ) : isLoading ? (
        <MediaGrid isLoading>{null}</MediaGrid>
      ) : items.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <EmptyState
            icon="library"
            title="No results"
            description="No items match the current rules."
          />
        </div>
      ) : (
        <MediaGrid>
          {items.map((item) => (
            <MediaCard key={item.id} item={item} showYear showType />
          ))}
        </MediaGrid>
      )}
    </div>
  );
}
