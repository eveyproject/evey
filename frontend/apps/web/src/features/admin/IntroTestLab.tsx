import { useMemo, useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import { FlaskConical, Play, Search } from "lucide-react";
import { eveyFetch, libraryServiceSearchMedia, useLibraryServiceListMediaItems } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { toast } from "@evey/store";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Spinner } from "@/components/ui/Spinner";
import { ICON } from "@/constants/iconSizes";
import { useDebounce } from "@/hooks/useDebounce";

interface DrySegment {
  type: string;
  startMs: number;
  endMs: number;
  confidence: number;
  source: string;
}

function fmtMs(ms: number): string {
  const s = Math.floor(ms / 1000);
  const m = Math.floor(s / 60);
  return `${m}:${String(s % 60).padStart(2, "0")}`;
}

// pickSampleEpisode returns the mid-season episode: first episodes often open
// with a longer cold open and finales drop the intro, the middle is the
// least surprising witness for a quick verdict.
export function pickSampleEpisode(episodes: V1MediaItem[]): V1MediaItem | null {
  if (episodes.length === 0) return null;
  return episodes[Math.floor(episodes.length / 2)];
}

const SEARCHABLE = ["MEDIA_ITEM_TYPE_SERIES", "MEDIA_ITEM_TYPE_MOVIE"];

export function IntroTestLab() {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const debounced = useDebounce(query, 300);
  const [picked, setPicked] = useState<V1MediaItem | null>(null);
  const [seasonId, setSeasonId] = useState("");
  const [episodeId, setEpisodeId] = useState("");
  const [result, setResult] = useState<{ segments: DrySegment[]; testedId: string } | null>(null);

  const { data: searchData, isFetching: searching } = useQuery({
    queryKey: ["/api/v1/media/search", "intro-test", debounced],
    queryFn: ({ signal }) => libraryServiceSearchMedia({ query: debounced, limit: 30 }, signal),
    enabled: debounced.length > 1 && !picked,
  });
  const candidates = (searchData?.items ?? []).filter((i) => SEARCHABLE.includes(i.type ?? ""));

  const isSeries = picked?.type === "MEDIA_ITEM_TYPE_SERIES";
  const { data: seasonsData } = useLibraryServiceListMediaItems(
    { parentId: picked?.id ?? "", "pagination.pageSize": 100 },
    { query: { enabled: !!picked && isSeries } } as never,
  );
  const seasons = useMemo(
    () => (seasonsData?.items ?? []).filter((s) => s.type === "MEDIA_ITEM_TYPE_SEASON"),
    [seasonsData],
  );
  const effectiveSeasonId = seasonId || seasons[0]?.id || "";

  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: effectiveSeasonId, "pagination.pageSize": 200 },
    { query: { enabled: !!effectiveSeasonId } } as never,
  );
  const episodes = useMemo(
    () => (episodesData?.items ?? []).filter((e) => e.type === "MEDIA_ITEM_TYPE_EPISODE"),
    [episodesData],
  );
  const sample = pickSampleEpisode(episodes);
  // The tested media: the picked episode override, else the mid-season
  // sample, else the movie itself.
  const testedId = isSeries ? (episodeId || sample?.id || "") : (picked?.id ?? "");

  const dryRun = useMutation({
    mutationFn: (mediaId: string) =>
      eveyFetch<DrySegment[]>({
        url: `/api/v1/admin/media/${encodeURIComponent(mediaId)}/detect-segments?dry_run=true`,
        method: "POST",
      }),
    onSuccess: (segments, mediaId) => setResult({ segments: segments ?? [], testedId: mediaId }),
    onError: (err: Error) => toast.error(err.message || t("admin.introTest.testFailed")),
  });

  // Apply persists for real, at the natural granularity: the SEASON for a
  // series (the cluster's unit), the movie itself otherwise.
  const applyId = isSeries ? effectiveSeasonId : (picked?.id ?? "");
  const apply = useMutation({
    mutationFn: (mediaId: string) =>
      eveyFetch<{ status: string }>({
        url: `/api/v1/admin/media/${encodeURIComponent(mediaId)}/detect-segments`,
        method: "POST",
      }),
    onSuccess: () => toast.success(t("admin.introTest.applyStarted")),
    onError: (err: Error) => toast.error(err.message || t("admin.introTest.testFailed")),
  });

  function reset() {
    setPicked(null);
    setSeasonId("");
    setEpisodeId("");
    setResult(null);
  }

  const introFound = (result?.segments ?? []).some((s) => s.type === "intro");

  return (
    <div className="max-w-3xl flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <FlaskConical size={ICON.lg} className="text-[var(--accent)]" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-[var(--text)]">{t("admin.introTest.title")}</h2>
      </div>
      <p className="text-sm text-[var(--text-muted)]">{t("admin.introTest.subtitle")}</p>

      {!picked ? (
        <div className="flex flex-col gap-2">
          <div className="relative">
            <Search size={ICON.sm} className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]" aria-hidden="true" />
            <Input
              autoFocus
              data-testid="intro-test-search"
              className="pl-9"
              placeholder={t("admin.introTest.searchPlaceholder")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          {searching && <Spinner size="sm" />}
          <ul className="flex flex-col divide-y divide-[var(--border)] rounded-[var(--radius-md)] border border-[var(--border)]">
            {candidates.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  className="w-full text-left px-3 py-2 hover:bg-[var(--surface)] text-sm text-[var(--text)] flex items-center justify-between"
                  onClick={() => setPicked(item)}
                >
                  <span className="truncate">{item.title}</span>
                  <span className="text-xs text-[var(--text-muted)] shrink-0 ml-2">
                    {item.type === "MEDIA_ITEM_TYPE_SERIES" ? t("media.itemType.series") : t("media.itemType.movie")}
                    {item.year ? ` · ${item.year}` : ""}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <div className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--surface)] p-4">
          <div className="flex items-center justify-between gap-2">
            <p className="font-semibold text-[var(--text)] truncate">{picked.title}</p>
            <button type="button" className="text-xs text-[var(--accent)] hover:underline shrink-0" onClick={reset}>
              {t("admin.introTest.changeMedia")}
            </button>
          </div>

          {isSeries && (
            <div className="flex flex-wrap gap-3">
              <Select
                label={t("admin.introTest.season")}
                data-testid="intro-test-season"
                value={effectiveSeasonId}
                onChange={(v) => { setSeasonId(v); setEpisodeId(""); setResult(null); }}
                options={seasons.map((s) => ({ label: s.title ?? s.id ?? "", value: s.id ?? "" }))}
              />
              <Select
                label={t("admin.introTest.sampleEpisode")}
                data-testid="intro-test-episode"
                value={testedId}
                onChange={(v) => { setEpisodeId(v); setResult(null); }}
                options={episodes.map((e) => ({
                  label: `${e.indexNumber != null ? `E${String(e.indexNumber).padStart(2, "0")} · ` : ""}${e.title ?? e.id}`,
                  value: e.id ?? "",
                }))}
              />
            </div>
          )}

          <div className="flex items-center gap-2">
            <Button
              data-testid="intro-test-run"
              disabled={!testedId || dryRun.isPending}
              onClick={() => testedId && dryRun.mutate(testedId)}
            >
              {dryRun.isPending ? t("admin.introTest.testing") : t("admin.introTest.test")}
            </Button>
            <Button
              variant="secondary"
              data-testid="intro-test-apply"
              disabled={!applyId || apply.isPending}
              onClick={() => applyId && apply.mutate(applyId)}
            >
              <Play size={ICON.sm} aria-hidden="true" />
              {t("admin.introTest.apply")}
            </Button>
          </div>
          {dryRun.isPending && (
            <p className="text-xs text-[var(--text-muted)]">{t("admin.introTest.testingHint")}</p>
          )}

          {result && !dryRun.isPending && (
            <div className="flex flex-col gap-2" data-testid="intro-test-result">
              <div className="flex items-center gap-2">
                {introFound ? (
                  <Badge data-testid="intro-test-verdict-found" variant="success">{t("admin.introTest.introFound")}</Badge>
                ) : (
                  <Badge data-testid="intro-test-verdict-none" variant="warning">{t("admin.introTest.noCandidate")}</Badge>
                )}
                <span className="text-xs text-[var(--text-muted)]">{t("admin.introTest.dryRunNote")}</span>
              </div>
              {result.segments.length > 0 && (
                <table className="text-sm text-[var(--text)]">
                  <thead>
                    <tr className="text-left text-xs text-[var(--text-muted)]">
                      <th className="pr-4 font-medium">{t("admin.introTest.colType")}</th>
                      <th className="pr-4 font-medium">{t("admin.introTest.colRange")}</th>
                      <th className="pr-4 font-medium">{t("admin.introTest.colSource")}</th>
                      <th className="font-medium">{t("admin.introTest.colConfidence")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.segments.map((s, i) => (
                      <tr key={i}>
                        <td className="pr-4">{s.type}</td>
                        <td className="pr-4 font-mono">{fmtMs(s.startMs)} → {fmtMs(s.endMs)}</td>
                        <td className="pr-4">{s.source}</td>
                        <td>{s.confidence.toFixed(2)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              <Link
                to="/admin/media/$mediaId"
                params={{ mediaId: result.testedId }}
                className="text-xs text-[var(--accent)] hover:underline"
              >
                {t("admin.introTest.openEditor")}
              </Link>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
