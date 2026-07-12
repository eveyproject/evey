import { useMemo, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Inbox, Film, Tv, Music } from "lucide-react";
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
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/requests")({
  component: RequestsPage,
});

type MediaType = "movie" | "tv" | "music";
type Tab = "search" | "mine";

const TYPES: { key: MediaType; labelKey: string; icon: React.ReactNode }[] = [
  { key: "movie", labelKey: "requests.typeMovie", icon: <Film size={15} /> },
  { key: "tv", labelKey: "requests.typeSeries", icon: <Tv size={15} /> },
  { key: "music", labelKey: "requests.typeMusic", icon: <Music size={15} /> },
];

const statusVariant: Record<string, "default" | "success" | "warning" | "error"> = {
  pending: "default",
  approved: "success",
  rejected: "error",
  later: "warning",
};

function candidateKey(c: { provider?: string; providerId?: string }) {
  return `${c.provider ?? ""}:${c.providerId ?? ""}`;
}

function SearchTab({ myRequests }: { myRequests: V1MediaRequestProto[] }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [mediaType, setMediaType] = useState<MediaType>("movie");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");

  const { data, isFetching, isError } = useMediaRequestServiceSearchTitles(
    { mediaType, query: submittedQuery },
    { query: { enabled: submittedQuery.trim().length > 0 } },
  );

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

  function handleRequest(candidate: V1TitleCandidate) {
    createRequest.mutate({
      data: {
        mediaType,
        provider: candidate.provider,
        providerId: candidate.providerId,
        title: candidate.title,
        year: candidate.year,
        posterUrl: candidate.posterUrl,
      },
    });
  }

  const results = data?.results ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        {TYPES.map((tp) => (
          <button
            key={tp.key}
            type="button"
            onClick={() => { setMediaType(tp.key); setSubmittedQuery(""); }}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast"
            style={{
              backgroundColor: mediaType === tp.key ? "var(--accent)" : "var(--surface-raised)",
              color: mediaType === tp.key ? "var(--filter-active-text)" : "var(--text-muted)",
            }}
            aria-pressed={mediaType === tp.key}
          >
            {tp.icon}
            {t(tp.labelKey)}
          </button>
        ))}
      </div>

      <div className="flex gap-2 items-end">
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
            const key = candidateKey(r);
            const alreadyRequested = requestedKeys.has(key);
            return (
              <li key={key} className="flex flex-col gap-2">
                <div
                  className="aspect-[2/3] rounded-[var(--radius-md)] overflow-hidden flex items-center justify-center text-xs text-[var(--text-muted)]"
                  style={{ backgroundColor: "var(--surface-raised)" }}
                >
                  {r.posterUrl ? (
                    <img src={r.posterUrl} alt="" className="w-full h-full object-cover" />
                  ) : (
                    "-"
                  )}
                </div>
                <div className="flex flex-col gap-1">
                  <p className="text-sm font-medium text-[var(--text)] line-clamp-2">
                    {r.title}
                    {r.year != null && r.year > 0 && (
                      <span className="text-[var(--text-muted)]"> {t("requests.year", { year: r.year })}</span>
                    )}
                  </p>
                  <Button
                    type="button"
                    variant={alreadyRequested ? "secondary" : "primary"}
                    disabled={alreadyRequested || createRequest.isPending}
                    onClick={() => handleRequest(r)}
                    className="w-full"
                  >
                    {alreadyRequested ? t("requests.requested") : t("requests.request")}
                  </Button>
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
        icon={<Inbox size={48} strokeWidth={1.5} />}
        title={t("requests.noRequests")}
      />
    );
  }

  return (
    <ul className="flex flex-col gap-2">
      {requests.map((r) => (
        <Card key={r.id} className="flex gap-3 items-start">
          {r.posterUrl ? (
            <img
              src={r.posterUrl}
              alt=""
              className="w-12 h-18 object-cover rounded-[var(--radius-sm)] flex-shrink-0"
              style={{ backgroundColor: "var(--surface-raised)" }}
            />
          ) : null}
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <span className="font-medium text-[var(--text)]">{r.title}</span>
              {r.year != null && r.year > 0 && (
                <span className="text-xs text-[var(--text-muted)]">{t("requests.year", { year: r.year })}</span>
              )}
              <Badge variant={statusVariant[r.status ?? "pending"] ?? "default"}>
                {t(`requests.status${capitalize(r.status)}`)}
              </Badge>
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

  const { data: policy, isLoading: policyLoading } = useMediaRequestServiceGetRequestPolicy();
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

  if (!policy?.canRequest) {
    return (
      <EmptyState
        icon={<Inbox size={48} strokeWidth={1.5} />}
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
