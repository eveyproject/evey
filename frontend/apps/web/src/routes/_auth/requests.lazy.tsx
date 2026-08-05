import { useMemo, useState } from "react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Inbox, Film, Tv, Music, Check } from "lucide-react";
import {
  useMediaRequestServiceGetRequestPolicy,
  useMediaRequestServiceSearchTitles,
  useMediaRequestServiceCreateRequest,
  useMediaRequestServiceListMyRequests,
  getMediaRequestServiceListMyRequestsQueryKey,
  type V1TitleCandidate,
  type V1MediaRequestProto,
} from "@evey/api";
import { toast } from "@evey/store";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { ICON } from "@/constants/iconSizes";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { RequestPoster } from "@/features/requests/RequestPoster";

export const Route = createLazyFileRoute("/_auth/requests")({
  component: RequestsPage,
});

type MediaType = "movie" | "tv" | "music";
// The search scopes the UI offers: movies and series are ONE scope searched
// together (two provider queries merged), music stays its own scope.
type SearchScope = "video" | "music";
type Tab = "search" | "mine";

const SCOPES: { key: SearchScope; labelKey: string; icon: React.ReactNode }[] = [
  { key: "video", labelKey: "requests.typeVideo", icon: <Film size={ICON.md} /> },
  { key: "music", labelKey: "requests.typeMusic", icon: <Music size={ICON.md} /> },
];

// A search candidate tagged with the concrete request type it will be
// submitted as - needed once movies and series share one result grid.
type TaggedCandidate = V1TitleCandidate & { requestType: MediaType };

// interleave merges two ranked lists preserving each one's own order
// (movie[0], tv[0], movie[1], tv[1], ...) so neither type buries the other -
// each provider list arrives sorted by its own relevance.
function interleave<T>(a: T[], b: T[]): T[] {
  const out: T[] = [];
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    if (i < a.length) out.push(a[i]);
    if (i < b.length) out.push(b[i]);
  }
  return out;
}

const statusVariant: Record<string, "default" | "success" | "warning" | "error"> = {
  pending: "default",
  approved: "success",
  rejected: "error",
  later: "warning",
};

// candidateKey includes the media type: with movies and series in one grid,
// a TMDB movie id and a TMDB tv id can collide numerically.
function candidateKey(c: { mediaType?: string; provider?: string; providerId?: string }) {
  return `${c.mediaType ?? ""}:${c.provider ?? ""}:${c.providerId ?? ""}`;
}

function SearchTab({ myRequests }: { myRequests: V1MediaRequestProto[] }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<SearchScope>("video");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");

  // Video scope fires BOTH provider searches and merges them; the server's
  // SearchTitles contract stays single-type. Music keeps its own single query.
  const searchEnabled = submittedQuery.trim().length > 0;
  const movieQ = useMediaRequestServiceSearchTitles(
    { mediaType: "movie", query: submittedQuery },
    { query: { enabled: searchEnabled && scope === "video" } },
  );
  const tvQ = useMediaRequestServiceSearchTitles(
    { mediaType: "tv", query: submittedQuery },
    { query: { enabled: searchEnabled && scope === "video" } },
  );
  const musicQ = useMediaRequestServiceSearchTitles(
    { mediaType: "music", query: submittedQuery },
    { query: { enabled: searchEnabled && scope === "music" } },
  );

  const isFetching = scope === "video" ? movieQ.isFetching || tvQ.isFetching : musicQ.isFetching;
  // One failing type must not sink the other's results: error only when
  // nothing at all can be shown.
  const isError = scope === "video" ? movieQ.isError && tvQ.isError : musicQ.isError;

  const results: TaggedCandidate[] = useMemo(() => {
    if (scope === "music") {
      return (musicQ.data?.results ?? []).map((r) => ({ ...r, requestType: "music" as const }));
    }
    return interleave<TaggedCandidate>(
      (movieQ.data?.results ?? []).map((r) => ({ ...r, requestType: "movie" as const })),
      (tvQ.data?.results ?? []).map((r) => ({ ...r, requestType: "tv" as const })),
    );
  }, [scope, movieQ.data, tvQ.data, musicQ.data]);

  const createRequest = useMediaRequestServiceCreateRequest({
    mutation: {
      onSuccess: () => {
        toast.success(t("requests.requestSubmitted"));
        void queryClient.invalidateQueries({ queryKey: getMediaRequestServiceListMyRequestsQueryKey() });
      },
      onError: () => toast.error(t("requests.requestFailed")),
    },
  });

  const requestedKeys = useMemo(
    () => new Set(myRequests.map((r) => candidateKey(r))),
    [myRequests],
  );

  function handleSearch() {
    setSubmittedQuery(query.trim());
  }

  function handleRequest(candidate: TaggedCandidate) {
    createRequest.mutate({
      data: {
        mediaType: candidate.requestType,
        provider: candidate.provider,
        providerId: candidate.providerId,
        title: candidate.title,
        year: candidate.year,
        posterUrl: candidate.posterUrl,
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        {SCOPES.map((sc) => (
          <button
            key={sc.key}
            type="button"
            onClick={() => { setScope(sc.key); setSubmittedQuery(""); }}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast"
            style={{
              backgroundColor: scope === sc.key ? "var(--accent)" : "var(--surface-raised)",
              color: scope === sc.key ? "var(--filter-active-text)" : "var(--text-muted)",
            }}
            aria-pressed={scope === sc.key}
          >
            {sc.icon}
            {t(sc.labelKey)}
          </button>
        ))}
      </div>

      <div className="flex flex-wrap gap-2 items-end">
        <div className="flex-1">
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                handleSearch();
              }
            }}
            placeholder={t("requests.searchPlaceholder")}
            aria-label={t("requests.searchPlaceholder")}
          />
        </div>
        <Button type="button" loading={isFetching} disabled={!query.trim()} onClick={handleSearch}>
          {t("requests.tabSearch")}
        </Button>
      </div>

      {isFetching ? (
        <div className="flex justify-center py-10">
          <Spinner />
        </div>
      ) : isError ? (
        <p className="text-sm text-[var(--text-muted)]">{t("requests.searchFailed")}</p>
      ) : submittedQuery === "" ? (
        <p className="text-sm text-[var(--text-muted)]">{t("requests.searchPrompt")}</p>
      ) : results.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)]">{t("requests.noResults")}</p>
      ) : (
        <ul className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
          {results.map((r) => {
            const key = candidateKey({ ...r, mediaType: r.requestType });
            const alreadyRequested = requestedKeys.has(key);
            return (
              <li key={key} className="flex flex-col gap-2">
                <div
                  className="relative aspect-[2/3] rounded-[var(--radius-md)] overflow-hidden flex items-center justify-center text-xs text-[var(--text-muted)]"
                  style={{ backgroundColor: "var(--surface-raised)" }}
                >
                  <RequestPoster posterUrl={r.posterUrl} title={r.title} mediaType={r.requestType} />
                  {r.inLibrary && (
                    <span
                      className="absolute bottom-1.5 left-1.5 right-1.5 flex items-center justify-center gap-1 px-1.5 py-0.5 rounded-[var(--radius-sm)] text-xs font-medium"
                      style={{ backgroundColor: "var(--overlay-bg)", color: "var(--text)" }}
                      data-testid="candidate-in-library"
                    >
                      <Check size={ICON.xs} aria-hidden="true" />
                      {t("requests.inLibrary")}
                    </span>
                  )}
                  {scope === "video" && (
                    <span
                      className="absolute top-1.5 left-1.5 flex items-center gap-1 px-1.5 py-0.5 rounded-[var(--radius-sm)] text-xs font-medium"
                      style={{ backgroundColor: "var(--overlay-bg)", color: "var(--text)" }}
                    >
                      {r.requestType === "tv" ? <Tv size={ICON.xs} aria-hidden="true" /> : <Film size={ICON.xs} aria-hidden="true" />}
                      {t(r.requestType === "tv" ? "requests.typeSeries" : "requests.typeMovie")}
                    </span>
                  )}
                </div>
                <div className="flex flex-col gap-1">
                  <p className="text-sm font-medium text-[var(--text)] line-clamp-2">
                    {r.title}
                    {r.year != null && r.year > 0 && (
                      <span className="text-[var(--text-muted)]"> {t("requests.year", { year: r.year })}</span>
                    )}
                  </p>
                  {r.inLibrary && r.mediaItemId ? (
                    // Already on the server: requesting it is refused server-side
                    // anyway, so offer the only useful action - go watch it.
                    <Link to="/media/$mediaId" params={{ mediaId: r.mediaItemId }} className="w-full">
                      <Button type="button" variant="secondary" className="w-full">
                        {t("requests.watchNow")}
                      </Button>
                    </Link>
                  ) : (
                    <Button
                      type="button"
                      variant={alreadyRequested ? "secondary" : "primary"}
                      disabled={alreadyRequested || createRequest.isPending}
                      onClick={() => handleRequest(r)}
                      className="w-full"
                    >
                      {alreadyRequested ? t("requests.requested") : t("requests.request")}
                    </Button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function MineTab({ requests, isLoading }: { requests: V1MediaRequestProto[]; isLoading: boolean }) {
  const { t } = useTranslation();

  if (isLoading) {
    return (
      <div className="flex justify-center py-10">
        <Spinner />
      </div>
    );
  }

  if (requests.length === 0) {
    return (
      <EmptyState
        icon={<Inbox size={ICON["4xl"]} strokeWidth={1.5} />}
        title={t("requests.noRequests")}
      />
    );
  }

  return (
    <ul className="flex flex-col gap-2">
      {requests.map((r) => (
        <Card key={r.id} className="flex gap-3 items-start">
          <div className="w-12 h-18 rounded-[var(--radius-sm)] overflow-hidden flex-shrink-0">
            <RequestPoster posterUrl={r.posterUrl} title={r.title} mediaType={r.mediaType} />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <span className="font-medium text-[var(--text)]">{r.title}</span>
              {r.year != null && r.year > 0 && (
                <span className="text-xs text-[var(--text-muted)]">{t("requests.year", { year: r.year })}</span>
              )}
              <Badge variant={statusVariant[r.status ?? "pending"] ?? "default"}>
                {t(`requests.status${capitalize(r.status)}`)}
              </Badge>
              {/* When the request was answered. The server has served answeredAt
                  since media requests shipped and no client ever showed it, so a
                  denied request gave no clue how old the decision was. Rendered
                  as a locale-formatted date rather than behind a new i18n key -
                  the status badge next to it already carries the wording. */}
              {r.answeredAt && (
                <span className="text-xs text-[var(--text-muted)]">
                  {new Date(r.answeredAt).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}
                </span>
              )}
            </div>
            {r.adminNote && (
              <p className="text-xs text-[var(--text-muted)] mt-1">{t("requests.adminNote", { note: r.adminNote })}</p>
            )}
          </div>
        </Card>
      ))}
    </ul>
  );
}

function capitalize(s?: string) {
  if (!s) return "Pending";
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function RequestsPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("search");

  const { data: policy, isLoading: policyLoading, isError: policyError, error: policyErr, refetch: refetchPolicy } = useMediaRequestServiceGetRequestPolicy();
  const { data: myRequestsResp, isLoading: mineLoading } = useMediaRequestServiceListMyRequests(
    undefined,
    { query: { enabled: policy?.canRequest === true } },
  );
  const myRequests = myRequestsResp?.items ?? [];

  if (policyLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  // Failing to read the policy is not the administrator having turned requests
  // off: saying so presented a network failure as a decision.
  if (policyError) {
    return <FetchErrorState error={policyErr} onRetry={() => void refetchPolicy()} />;
  }

  if (!policy?.canRequest) {
    return (
      <EmptyState
        icon={<Inbox size={ICON["4xl"]} strokeWidth={1.5} />}
        title={t("requests.disabledTitle")}
        description={t("requests.disabledDesc")}
      />
    );
  }

  return (
    <div className="w-full max-w-4xl mx-auto px-4 sm:px-6 py-6 sm:py-8 flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("requests.title")}</h1>
        <p className="text-sm text-[var(--text-muted)] mt-1">{t("requests.subtitle")}</p>
      </div>

      <div className="flex gap-1 border-b border-[var(--border)]">
        {(["search", "mine"] as Tab[]).map((tb) => (
          <button
            key={tb}
            onClick={() => setTab(tb)}
            className={`px-4 py-2 text-sm transition-colors-fast border-b-2 -mb-px ${
              tab === tb
                ? "border-[var(--accent)] text-[var(--accent)]"
                : "border-transparent text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {t(tb === "search" ? "requests.tabSearch" : "requests.tabMine")}
          </button>
        ))}
      </div>

      {tab === "search" ? (
        <SearchTab myRequests={myRequests} />
      ) : (
        <MineTab requests={myRequests} isLoading={mineLoading} />
      )}
    </div>
  );
}
