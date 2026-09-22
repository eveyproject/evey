import { useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  getMediaRequestServiceListMyRequestsQueryKey,
  useMediaRequestServiceCreateRequest,
  useMediaRequestServiceGetRequestPolicy,
  useMediaRequestServiceListMyRequests,
  useMediaRequestServiceSearchTitles,
} from "@evey/api";
import { toast } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Button, Card, Spinner, TextField } from "../components/ui";
import { navigate } from "../app/router";

type Scope = "video" | "music";
type RequestType = "movie" | "tv" | "music";
interface Candidate {
  provider?: string;
  providerId?: string;
  title?: string;
  year?: number;
  posterUrl?: string;
  inLibrary?: boolean;
  mediaItemId?: string;
  requestType: RequestType;
}

// The two provider searches a video scope fires are merged one for one, so
// neither films nor series own the top of the list.
function interleave<T>(a: T[], b: T[]): T[] {
  const out: T[] = [];
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    if (i < a.length) out.push(a[i]);
    if (i < b.length) out.push(b[i]);
  }
  return out;
}

function candidateKey(c: { mediaType?: string; provider?: string; providerId?: string }): string {
  return `${c.mediaType ?? ""}:${c.provider ?? ""}:${c.providerId ?? ""}`;
}

/** Ask for a title the library does not have, and see what became of the asks. */
export function RequestsScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [tab, setTab] = useState<"search" | "mine">("search");
  const [scope, setScope] = useState<Scope>("video");
  const [query, setQuery] = useState("");
  const [submitted, setSubmitted] = useState("");

  const { data: policy, isLoading: policyLoading } = useMediaRequestServiceGetRequestPolicy();
  const canRequest = policy?.canRequest === true;

  const { data: mineResp, isLoading: mineLoading } = useMediaRequestServiceListMyRequests(undefined, { query: { enabled: canRequest } });
  const mine = mineResp?.items ?? [];
  const requested = useMemo(() => new Set(mine.map((r) => candidateKey(r))), [mine]);

  const enabled = canRequest && submitted.trim().length > 0;
  const movies = useMediaRequestServiceSearchTitles({ mediaType: "movie", query: submitted }, { query: { enabled: enabled && scope === "video" } });
  const series = useMediaRequestServiceSearchTitles({ mediaType: "tv", query: submitted }, { query: { enabled: enabled && scope === "video" } });
  const music = useMediaRequestServiceSearchTitles({ mediaType: "music", query: submitted }, { query: { enabled: enabled && scope === "music" } });
  const searching = scope === "video" ? movies.isFetching || series.isFetching : music.isFetching;

  const results: Candidate[] = useMemo(() => {
    if (scope === "music") return (music.data?.results ?? []).map((r) => ({ ...r, requestType: "music" as const }));
    return interleave<Candidate>(
      (movies.data?.results ?? []).map((r) => ({ ...r, requestType: "movie" as const })),
      (series.data?.results ?? []).map((r) => ({ ...r, requestType: "tv" as const })),
    );
  }, [scope, movies.data, series.data, music.data]);

  const create = useMediaRequestServiceCreateRequest({
    mutation: {
      onSuccess: () => {
        toast.success(t("requests.requestSubmitted"));
        void qc.invalidateQueries({ queryKey: getMediaRequestServiceListMyRequestsQueryKey() });
      },
      onError: () => toast.error(t("requests.requestFailed")),
    },
  });

  if (policyLoading) return <div className="screen screen-center"><Spinner /></div>;
  if (!canRequest) {
    return (
      <div className="screen screen-center">
        <h1 className="title">{t("requests.disabledTitle")}</h1>
        <p className="muted">{t("requests.disabledDesc")}</p>
      </div>
    );
  }

  return (
    <div className="screen">
      <FocusRoot autoFocus className="header">
        <h1 className="title">{t("common.nav.requests", "Requests")}</h1>
        <div className="actions" style={{ margin: 0 }}>
          <Button variant={tab === "search" ? "primary" : "default"} onPress={() => setTab("search")}>{t("common.nav.search")}</Button>
          <Button variant={tab === "mine" ? "primary" : "default"} onPress={() => setTab("mine")}>{t("requests.tabMine")}</Button>
        </div>
      </FocusRoot>

      {tab === "search" ? (
        <>
          <FocusRoot className="chips">
            <TextField value={query} onChange={setQuery} placeholder={t("requests.searchPlaceholder")} onSubmit={() => setSubmitted(query)} />
            <Button variant={scope === "video" ? "primary" : "default"} onPress={() => setScope("video")}>{t("media.search.sectionMedia")}</Button>
            <Button variant={scope === "music" ? "primary" : "default"} onPress={() => setScope("music")}>{t("media.search.sectionMusic")}</Button>
            <Button onPress={() => setSubmitted(query)}>{t("common.nav.search")}</Button>
          </FocusRoot>
          {searching ? <Spinner /> : null}
          {!searching && submitted && results.length === 0 ? <p className="muted">{t("requests.noResults")}</p> : null}
          {!submitted ? <p className="muted">{t("requests.searchPrompt")}</p> : null}
          <FocusRoot>
            <div className="grid">
              {results.map((c) => {
                const already = requested.has(candidateKey({ ...c, mediaType: c.requestType }));
                const label = `${c.title ?? ""}${c.year ? ` (${c.year})` : ""}`;
                return (
                  <Card
                    key={`${c.provider}-${c.providerId}-${c.requestType}`}
                    title={`${label}${c.inLibrary ? ` · ${t("requests.inLibrary")}` : already ? ` · ${t("requests.requested")}` : ""}`}
                    imageUrl={c.posterUrl}
                    onPress={() => {
                      if (c.inLibrary && c.mediaItemId) { navigate({ name: "media", mediaId: c.mediaItemId }); return; }
                      if (already) return;
                      create.mutate({ data: { mediaType: c.requestType, provider: c.provider, providerId: c.providerId, title: c.title, year: c.year, posterUrl: c.posterUrl } });
                    }}
                  />
                );
              })}
            </div>
          </FocusRoot>
        </>
      ) : (
        <>
          {mineLoading ? <Spinner /> : null}
          {!mineLoading && mine.length === 0 ? <p className="muted">{t("requests.noRequests")}</p> : null}
          <FocusRoot>
            <div className="track-list">
              {mine.map((r, i) => (
                <Button key={r.id ?? i} autoFocus={i === 0} onPress={() => { /* a request is a record, not a destination */ }}>
                  <span className="track-title">{r.title}{r.year ? ` (${r.year})` : ""}</span>
                  <span className="muted">{r.status ?? ""}{r.adminNote ? ` · ${t("requests.adminNote")}: ${r.adminNote}` : ""}</span>
                </Button>
              ))}
            </div>
          </FocusRoot>
        </>
      )}
    </div>
  );
}
