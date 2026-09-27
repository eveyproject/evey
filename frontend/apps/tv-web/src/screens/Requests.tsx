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
import { Pressable, Spinner, TextField } from "../components/ui";
import { Icon } from "../components/icons";
import type { IoniconName } from "../components/ionicons";
import { ScreenState } from "../components/ScreenState";
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

const SCOPES: { key: Scope; labelKey: string; icon: IoniconName }[] = [
  { key: "video", labelKey: "requests.typeVideo", icon: "film-outline" },
  { key: "music", labelKey: "requests.typeMusic", icon: "musical-notes-outline" },
];

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

function statusClass(status?: string): string {
  return status === "approved" ? "ok" : status === "rejected" ? "ko" : status === "later" ? "later" : "";
}

function capitalize(s?: string): string {
  if (!s) return "Pending";
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function Thumb({ url, inLibrary }: { url?: string; inLibrary?: boolean }) {
  return (
    <span className="result-thumb">
      {url ? <img src={url} alt="" loading="lazy" /> : "?"}
      {inLibrary ? <span className="in-library"><Icon name="checkmark-circle" size={16} /></span> : null}
    </span>
  );
}

/** Ask for a title the library does not have, and see what became of the asks. */
export function RequestsScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [tab, setTab] = useState<"search" | "mine">("search");
  const [scope, setScope] = useState<Scope>("video");
  const [query, setQuery] = useState("");
  const [submitted, setSubmitted] = useState("");

  const policy = useMediaRequestServiceGetRequestPolicy();
  const canRequest = policy.data?.canRequest === true;
  const { data: mineResp, isLoading: mineLoading } = useMediaRequestServiceListMyRequests(undefined, { query: { enabled: canRequest } });
  const mine = useMemo(() => mineResp?.items ?? [], [mineResp?.items]);
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

  if (policy.isLoading || policy.isError) return <ScreenState loading={policy.isLoading} error={policy.isError} onRetry={() => void policy.refetch()} />;
  if (!canRequest) {
    return (
      <FocusRoot autoFocus className="center-fill requests-off">
        <span className="muted"><Icon name="mail-unread-outline" size={64} /></span>
        <p className="requests-off-title">{t("requests.disabledTitle")}</p>
        <p className="requests-off-desc">{t("requests.disabledDesc")}</p>
        <Pressable className="tv-card empty-action" autoFocus onPress={() => navigate({ name: "libraries" })}>{t("media.library.libraries")}</Pressable>
      </FocusRoot>
    );
  }

  return (
    <FocusRoot autoFocus className="tab-page requests-page">
      <h1 className="tab-title">{t("requests.title")}</h1>
      <FocusRoot className="req-tabs">
        <Pressable className={`req-tab ${tab === "search" ? "active" : ""}`} autoFocus onPress={() => setTab("search")}>{t("requests.tabSearch")}</Pressable>
        <Pressable className={`req-tab ${tab === "mine" ? "active" : ""}`} onPress={() => setTab("mine")}>{t("requests.tabMine")}</Pressable>
      </FocusRoot>

      {tab === "search" ? (
        <>
          <FocusRoot className="req-chips">
            {SCOPES.map((sc) => (
              <Pressable key={sc.key} className={`req-chip ${scope === sc.key ? "active" : ""}`} onPress={() => { setScope(sc.key); setSubmitted(""); }}>
                <Icon name={sc.icon} size={18} /><span>{t(sc.labelKey)}</span>
              </Pressable>
            ))}
          </FocusRoot>
          <FocusRoot className="req-search">
            <TextField value={query} onChange={setQuery} placeholder={t("requests.searchPlaceholder")} onSubmit={() => setSubmitted(query.trim())} />
          </FocusRoot>
          {searching ? (
            <div className="center-fill pad-top"><Spinner /></div>
          ) : submitted === "" ? (
            <div className="center-fill pad-top"><p className="req-hint">{t("requests.searchPrompt")}</p></div>
          ) : results.length === 0 ? (
            <div className="center-fill pad-top"><p className="req-hint">{t("requests.noResults")}</p></div>
          ) : (
            <FocusRoot className="req-list">
              {results.map((c, i) => {
                const already = requested.has(candidateKey({ ...c, mediaType: c.requestType }));
                return (
                  <div className="req-row" key={candidateKey({ ...c, mediaType: c.requestType })}>
                    <Thumb url={c.posterUrl} inLibrary={c.inLibrary} />
                    <div className="req-info">
                      <span className="req-title">{c.title}{c.year != null && c.year > 0 ? ` (${c.year})` : ""}</span>
                      {scope === "video" ? (
                        <span className="req-type">
                          <Icon name={c.requestType === "tv" ? "tv-outline" : "film-outline"} size={14} />
                          {t(c.requestType === "tv" ? "requests.typeSeries" : "requests.typeMovie")}
                        </span>
                      ) : null}
                      {c.inLibrary ? <span className="req-note">{t("requests.inLibrary")}</span> : null}
                    </div>
                    {c.inLibrary && c.mediaItemId ? (
                      <Pressable className="tv-card req-action" autoFocus={i === 0} onPress={() => navigate({ name: "media", mediaId: c.mediaItemId! })}>
                        {t("requests.watchNow")}
                      </Pressable>
                    ) : (
                      <Pressable
                        className={`tv-card req-action ${already ? "" : "primary"}`}
                        autoFocus={i === 0}
                        onPress={() => {
                          if (already || create.isPending) return;
                          create.mutate({ data: { mediaType: c.requestType, provider: c.provider, providerId: c.providerId, title: c.title, year: c.year, posterUrl: c.posterUrl } });
                        }}
                      >
                        {already ? t("requests.requested") : t("requests.request")}
                      </Pressable>
                    )}
                  </div>
                );
              })}
            </FocusRoot>
          )}
        </>
      ) : mineLoading ? (
        <div className="center-fill pad-top"><Spinner /></div>
      ) : mine.length === 0 ? (
        <div className="center-fill pad-top"><p className="req-hint">{t("requests.noRequests")}</p></div>
      ) : (
        <FocusRoot className="req-list">
          {mine.map((r, i) => (
            <Pressable key={r.id ?? i} className="tv-card req-row mine" autoFocus={i === 0} label={r.title ?? t("requests.title")} onPress={() => {}}>
              <Thumb url={r.posterUrl} />
              <span className="req-info">
                <span className="req-title">{r.title}{r.year != null && r.year > 0 ? ` (${r.year})` : ""}</span>
                <span className={`req-status ${statusClass(r.status)}`}>{t(`requests.status${capitalize(r.status)}`)}</span>
                {r.adminNote ? <span className="req-note">{t("requests.adminNote", { note: r.adminNote })}</span> : null}
              </span>
            </Pressable>
          ))}
        </FocusRoot>
      )}
    </FocusRoot>
  );
}
