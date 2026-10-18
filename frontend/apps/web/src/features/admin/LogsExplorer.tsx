import { memo, useState, useEffect, useLayoutEffect, useMemo, useCallback, useRef } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useTranslation } from "react-i18next";
import { Search, Radio, ChevronRight, Copy, SlidersHorizontal, AlertTriangle, ArrowUp } from "lucide-react";
import { eveyFetch, getBaseURL, useUserServiceListUsers, useUserServiceListAllProfiles } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { Select } from "@/components/ui/Select";
import { Input } from "@/components/ui/Input";
import { FilterChip } from "@/components/ui/FilterChip";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { EmptyState } from "@/components/ui/EmptyState";
import { ICON } from "@/constants/iconSizes";
import { buildIdentityIndex, labelFor, resolveUserFilter, type IdentityIndex } from "./logIdentities";
import { useMediaLabels, type MediaLabelIndex } from "@evey/client";
import {
  type LogEntry,
  type LogSearchResponse,
  WHERE_VALUES,
  SOURCE_VALUES,
  parseLogLine,
  isAccessLog,
} from "./logsClassify";

const LEVELS = ["ERROR", "WARN", "INFO", "DEBUG"] as const;
const PAGE_SIZE = 100;
const LIVE_CAP = 1000;
// Collapsed-row estimate for the virtualizer; expanded rows are re-measured
// via measureElement (variable height once a row is toggled open).
const ROW_HEIGHT_ESTIMATE = 40;

interface FilterParams {
  levels: string[];
  where: string;
  source: string;
  q: string;
  sessionId: string;
  userId: string;
  mediaId: string;
  // Show the gRPC interceptor's per-call access lines of SUCCESSFUL calls.
  // Off by default: on a live server they are four lines in five (heartbeats,
  // dashboard polls) and were all the operator saw in the live tail. Picking
  // the grpc source explicitly also shows them.
  showAccess: boolean;
}

// hidesAccess is the one predicate both the server query and the live-tail
// client filter derive from.
function hidesAccess(f: FilterParams): boolean {
  return !f.showAccess && f.source !== "grpc";
}

function levelVariant(level: string): "error" | "warning" | "info" | "default" {
  switch (level) {
    case "ERROR":
      return "error";
    case "WARN":
      return "warning";
    case "INFO":
      return "info";
    default:
      return "default";
  }
}

function levelBorder(level: string): string {
  switch (level) {
    case "ERROR":
      return "var(--badge-error-text,#fca5a5)";
    case "WARN":
      return "var(--badge-warning-text,#fde047)";
    case "INFO":
      return "var(--badge-info-text,#93c5fd)";
    default:
      return "var(--border)";
  }
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString(undefined, { hour12: false }) + "." + String(d.getMilliseconds()).padStart(3, "0");
}

function buildSearchQS(f: FilterParams, limit: number, offset: number): string {
  const p = new URLSearchParams();
  if (f.levels.length) p.set("level", f.levels.join(","));
  if (f.where) p.set("where", f.where);
  if (f.source) p.set("source", f.source);
  if (f.q) p.set("q", f.q);
  if (f.sessionId) p.set("session_id", f.sessionId);
  if (f.userId) p.set("user_id", f.userId);
  if (f.mediaId) p.set("media_id", f.mediaId);
  if (hidesAccess(f)) p.set("hide_access", "1");
  p.set("limit", String(limit));
  p.set("offset", String(offset));
  return p.toString();
}

// matchClient mirrors the server matchLog predicate, used to filter the live
// SSE stream (which delivers unfiltered raw lines).
function matchClient(e: LogEntry, f: FilterParams): boolean {
  if (f.levels.length && !f.levels.includes(e.level)) return false;
  if (f.where && e.where !== f.where) return false;
  if (f.source && e.source !== f.source) return false;
  if (f.sessionId && e.sessionId !== f.sessionId) return false;
  if (f.userId && e.userId !== f.userId) return false;
  if (f.mediaId && e.mediaId !== f.mediaId) return false;
  if (f.q && !e.raw.toLowerCase().includes(f.q.toLowerCase())) return false;
  if (hidesAccess(f) && isAccessLog(e)) return false;
  return true;
}

// Memoised: in live-tail mode every incoming SSE line replaces the whole
// entries array (up to LIVE_CAP=1000). Each row's `entry` object is stable once
// created, so memo lets an unchanged row skip re-render instead of re-rendering
// all ~1000 on every tick.
// `who` resolves user/profile ids to names (see logIdentities.ts). It is a
// prop, not a hook call: the row is memoised per entry and the index changes
// only when the account listing does.
// `open` is owned by the explorer, not the row: the live tail anchors the
// viewport on an expanded row as new lines pile up above it, so the parent has
// to know which rows are open.
// `media` names the title behind a media id ("Some Series S01E01 - Pilot"):
// an episode called "Pilot" is every series' first episode.
const LogRow = memo(function LogRow({
  entry,
  who,
  media,
  open,
  onToggle,
}: {
  entry: LogEntry;
  who: IdentityIndex;
  media: MediaLabelIndex;
  open: boolean;
  onToggle: (uid: string) => void;
}) {
  const { t } = useTranslation();
  const mediaLabel = entry.mediaId ? media.get(entry.mediaId) : undefined;
  const copy = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      void navigator.clipboard?.writeText(entry.raw);
    },
    [entry.raw],
  );
  const ids: Array<[string, string | undefined]> = [
    ["media", entry.mediaId],
    ["session", entry.sessionId],
    ["user", entry.userId && labelFor(who, entry.userId)],
  ];
  // The name rides the collapsed line too: "who" is the first question an
  // operator asks of a line, and expanding each one to find out is what made
  // the explorer useless for a tester's report.
  const userName = entry.userId ? who.get(entry.userId) : undefined;
  return (
    <div
      className="border-b border-[var(--border)] text-sm"
      style={{ borderLeft: `3px solid ${levelBorder(entry.level)}` }}
    >
      <button
        type="button"
        onClick={() => onToggle(entry.uid)}
        className="w-full flex items-start gap-2 px-3 py-1.5 text-left hover:bg-[var(--surface-raised)] transition-colors-fast"
        aria-expanded={open}
      >
        <ChevronRight
          size={ICON.sm}
          className="mt-0.5 shrink-0 text-[var(--text-muted)] transition-transform-fast"
          style={{ transform: open ? "rotate(90deg)" : "none" }}
          aria-hidden="true"
        />
        <span
          className="shrink-0 font-mono text-xs text-[var(--text-muted)] tabular-nums"
          title={entry.time}
        >
          {fmtTime(entry.time)}
        </span>
        <Badge variant={levelVariant(entry.level)}>{entry.level || "?"}</Badge>
        <Badge>{entry.where}</Badge>
        {entry.source && entry.source !== "other" && <Badge>{entry.source}</Badge>}
        {userName && (
          <span className="shrink-0 text-xs text-[var(--text-muted)]" title={entry.userId}>
            {userName}
          </span>
        )}
        {mediaLabel && (
          <span
            className="max-w-[18rem] shrink-0 truncate text-xs text-[var(--accent)]"
            title={`${t(`admin.logs.mediaKind.${mediaLabel.kind}`)} · ${entry.mediaId}`}
            data-testid="log-media-label"
          >
            {mediaLabel.label}
          </span>
        )}
        <span className="min-w-0 flex-1 truncate text-[var(--text)]">{entry.msg}</span>
      </button>
      {open && (
        <div className="px-9 pb-3 pt-1 flex flex-col gap-2">
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]">
            {entry.component && (
              <span>
                <span className="opacity-60">component:</span> {entry.component}
              </span>
            )}
            {mediaLabel && (
              <span>
                <span className="opacity-60">{t(`admin.logs.mediaKind.${mediaLabel.kind}`)}:</span> {mediaLabel.label}
              </span>
            )}
            {ids.map(([k, v]) =>
              v ? (
                <span key={k}>
                  <span className="opacity-60">{k}:</span> {v}
                </span>
              ) : null,
            )}
            {entry.requestId && (
              <span>
                <span className="opacity-60">request_id:</span> {entry.requestId}
              </span>
            )}
          </div>
          {entry.fields && Object.keys(entry.fields).length > 0 && (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-0.5 font-mono text-xs">
              {Object.entries(entry.fields).map(([k, v]) => (
                <div key={k} className="flex gap-2 break-all">
                  <span className="text-[var(--accent)] shrink-0">{k}</span>
                  <span className="text-[var(--text)]">{typeof v === "string" ? labelFor(who, v) : JSON.stringify(v)}</span>
                </div>
              ))}
            </div>
          )}
          <button
            type="button"
            onClick={copy}
            className="self-start flex items-center gap-1 text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast"
          >
            <Copy size={ICON.xs} aria-hidden="true" />
            {t("admin.logs.copyRaw")}
          </button>
        </div>
      )}
    </div>
  );
});

export function LogsExplorer() {
  const { t } = useTranslation();
  const [levels, setLevels] = useState<string[]>([]);
  const [where, setWhere] = useState("");
  const [source, setSource] = useState("");
  const [qInput, setQInput] = useState("");
  const [q, setQ] = useState("");
  const [sessionId, setSessionId] = useState("");
  const [userId, setUserId] = useState("");
  const [mediaId, setMediaId] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  // Live tail on by default: the operator opening Logs & Debug is almost
  // always chasing something happening NOW - land on the stream, not on a
  // search over history (toggle off for forensic queries).
  const [live, setLive] = useState(true);
  const [liveEntries, setLiveEntries] = useState<LogEntry[]>([]);
  const [showAccess, setShowAccess] = useState(false);
  // Which rows are expanded, by entry uid. Lives here (not in the row) because
  // an open row is what pins the live viewport - see the anchor below.
  const [openUids, setOpenUids] = useState<ReadonlySet<string>>(() => new Set());
  const toggleOpen = useCallback((uid: string) => {
    setOpenUids((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) next.delete(uid);
      else next.add(uid);
      return next;
    });
  }, []);
  // Live lines that arrived above an anchored viewport and are therefore out
  // of sight; surfaced as a "N new lines" pill that jumps back to the top.
  const [hiddenAbove, setHiddenAbove] = useState(0);
  // The entry the live viewport is pinned to, if any (see the anchor block
  // further down). Declared here because the SSE effect consults it.
  const anchorRef = useRef<{ uid: string; delta: number } | null>(null);

  // Debounce the free-text box so we don't refetch on every keystroke.
  useEffect(() => {
    const id = setTimeout(() => setQ(qInput.trim()), 300);
    return () => clearTimeout(id);
  }, [qInput]);

  // Names for the ids the lines carry, from the admin listings this page
  // already has the right to read. Missing data (still loading, RPC down) just
  // leaves the ids bare - the explorer must never depend on it.
  const { data: userData } = useUserServiceListUsers();
  const { data: profileData } = useUserServiceListAllProfiles();
  const users = useMemo(() => userData?.users ?? [], [userData]);
  const who = useMemo(() => buildIdentityIndex(users, profileData?.profiles ?? []), [users, profileData]);

  const filters: FilterParams = useMemo(
    () => ({
      levels, where, source, q,
      sessionId: sessionId.trim(),
      userId: resolveUserFilter(users, userId),
      mediaId: mediaId.trim(),
      showAccess,
    }),
    [levels, where, source, q, sessionId, userId, mediaId, users, showAccess],
  );

  const { data, isLoading, isError, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ["admin-logs-search", filters],
    enabled: !live,
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      eveyFetch<LogSearchResponse>({
        url: `/api/v1/admin/logs/search?${buildSearchQS(filters, PAGE_SIZE, pageParam)}`,
        method: "GET",
        signal,
        // An older server answered a search that matched nothing with entries: null.
      }).then((res) => ({ ...res, entries: res.entries ?? [] })),
    getNextPageParam: (last, all) => {
      // An empty page ends the walk even when total is higher: the server
      // clamps how deep a single search can page, and `fetched < total` alone
      // would keep re-requesting the same offset forever once that floor is
      // reached (the count never grows, so the condition never flips).
      if (last.entries.length === 0) return undefined;
      const fetched = all.reduce((n, p) => n + p.entries.length, 0);
      return fetched < last.total ? fetched : undefined;
    },
  });

  // Live tail: stream the SSE endpoint over fetch and prepend classified,
  // filtered lines. fetch (not EventSource) on purpose: EventSource cannot
  // send headers, which forced the ADMIN access token into the URL - straight
  // into reverse-proxy logs and browser history. The raw fetch is one of the
  // sanctioned non-eveyFetch sites (streaming body); the Bearer is re-read on
  // every (re)connect so a token rotation doesn't kill the tail.
  useEffect(() => {
    if (!live) return;
    setLiveEntries([]);
    setOpenUids(new Set());
    setHiddenAbove(0);
    anchorRef.current = null;
    const ctrl = new AbortController();
    const handleLine = (line: string) => {
      if (!line.startsWith("data:")) return;
      const entry = parseLogLine(line.slice(5).trimStart());
      if (!entry || !matchClient(entry, filters)) return;
      setLiveEntries((prev) => [entry, ...prev].slice(0, LIVE_CAP));
      if (anchorRef.current) setHiddenAbove((n) => n + 1);
    };
    void (async () => {
      while (!ctrl.signal.aborted) {
        try {
          const token = useAuthStore.getState().accessToken;
          const res = await fetch(`${getBaseURL()}/api/v1/admin/logs/stream?lines=100`, {
            headers: token ? { Authorization: `Bearer ${token}` } : undefined,
            signal: ctrl.signal,
          });
          if (!res.ok || !res.body) throw new Error(`stream http ${res.status}`);
          const reader = res.body.getReader();
          const dec = new TextDecoder();
          let buf = "";
          for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            buf += dec.decode(value, { stream: true });
            const frames = buf.split("\n\n");
            buf = frames.pop() ?? "";
            for (const frame of frames) frame.split("\n").forEach(handleLine);
          }
        } catch {
          /* aborted (toggle off) or transient network error */
        }
        if (ctrl.signal.aborted) return;
        // mirror EventSource's auto-reconnect
        await new Promise((r) => setTimeout(r, 2000));
      }
    })();
    return () => ctrl.abort();
  }, [live, filters]);

  // Search pages are append-only, so page+row position IS a stable identity
  // here - unlike the live list, where prepending shifts every index. Server
  // entries carry no uid of their own, so one is stamped on at flatten time.
  const searchEntries = useMemo(
    () =>
      data?.pages.flatMap((p, pageIdx) =>
        p.entries.map((e, rowIdx) => (e.uid ? e : { ...e, uid: `page-${pageIdx}-${rowIdx}` })),
      ) ?? [],
    [data],
  );
  const entries = live ? liveEntries : searchEntries;
  const total = live ? liveEntries.length : (data?.pages[0]?.total ?? 0);
  const truncated = !live && (data?.pages[0]?.truncated ?? false);

  // The entry list is virtualized (long searches keep every page mounted in
  // the DOM otherwise), so infinite scroll can no longer rely on a sentinel
  // element at the bottom of the (now windowed) DOM - it watches the last
  // *virtual* item instead, which is the standard tanstack-virtual pattern.
  const parentRef = useRef<HTMLDivElement>(null);
  // getItemKey ties both React's reconciliation and the virtualizer's size
  // cache to the entry's own uid rather than to its index. In live tail every
  // new line is PREPENDED, so index N holds a different entry one tick later:
  // keyed by index, an expanded row kept its open state (and its measured
  // height) while its content slid to the next log under the reader's eyes.
  const getItemKey = useCallback((index: number) => entries[index]?.uid ?? index, [entries]);
  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT_ESTIMATE,
    getItemKey,
    overscan: 8,
  });
  const virtualItems = virtualizer.getVirtualItems();
  // Only the ids on screen are resolved: a live tail holds up to a thousand
  // lines, and the window is what the operator reads.
  const visibleMediaIds = useMemo(
    () => virtualItems.map((v) => entries[v.index]?.mediaId ?? "").filter(Boolean),
    [virtualItems, entries],
  );
  const mediaLabels = useMediaLabels(visibleMediaIds);

  useEffect(() => {
    if (live) return;
    const last = virtualItems[virtualItems.length - 1];
    if (!last) return;
    if (last.index >= entries.length - 1 && hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [live, virtualItems, entries.length, hasNextPage, isFetchingNextPage, fetchNextPage]);

  // Live-tail viewport anchor. New lines are PREPENDED, so without help every
  // arrival slides the whole list down by one row under the reader's eyes -
  // the one thing that made an expanded entry impossible to read or copy. The
  // viewport follows the stream only while it sits at the top with nothing
  // expanded; otherwise it is pinned to the entry it was on (`uid` plus how far
  // into it the viewport had scrolled) and re-placed after every render from
  // the virtualizer's measurements, which also absorbs the estimate-to-measured
  // height corrections of the rows piling up above.
  const captureAnchor = useCallback(() => {
    const el = parentRef.current;
    if (!el || !live) return;
    if (el.scrollTop <= 0 && openUids.size === 0) {
      anchorRef.current = null;
      setHiddenAbove(0);
      return;
    }
    const item = virtualizer.measurementsCache.find((m) => m.end > el.scrollTop);
    if (!item) return;
    anchorRef.current = { uid: String(item.key), delta: el.scrollTop - item.start };
  }, [live, openUids, virtualizer]);
  // Expanding a row while following the stream is what starts the pinning.
  useEffect(() => {
    if (openUids.size > 0 && !anchorRef.current) captureAnchor();
    if (openUids.size === 0 && parentRef.current && parentRef.current.scrollTop <= 0) {
      anchorRef.current = null;
      setHiddenAbove(0);
    }
  }, [openUids, captureAnchor]);
  useLayoutEffect(() => {
    const anchor = anchorRef.current;
    const el = parentRef.current;
    if (!live || !anchor || !el) return;
    const index = entries.findIndex((e) => e.uid === anchor.uid);
    if (index < 0) {
      // Fell off the LIVE_CAP window: nothing left to pin to.
      anchorRef.current = null;
      return;
    }
    const m = virtualizer.measurementsCache[index];
    if (!m) return;
    const want = m.start + anchor.delta;
    if (Math.abs(el.scrollTop - want) > 1) el.scrollTop = want;
  });
  const jumpToTop = useCallback(() => {
    anchorRef.current = null;
    setHiddenAbove(0);
    if (parentRef.current) parentRef.current.scrollTop = 0;
  }, []);

  const toggleLevel = useCallback((l: string) => {
    setLevels((prev) => (prev.includes(l) ? prev.filter((x) => x !== l) : [...prev, l]));
  }, []);

  const whereOptions = useMemo(
    () => [{ value: "", label: t("admin.logs.where.all") }, ...WHERE_VALUES.map((w) => ({ value: w, label: t(`admin.logs.where.${w}`) }))],
    [t],
  );
  const sourceOptions = useMemo(
    () => [{ value: "", label: t("admin.logs.source.all") }, ...SOURCE_VALUES.map((s) => ({ value: s, label: t(`admin.logs.source.${s}`) }))],
    [t],
  );

  return (
    <div className="flex flex-col gap-4">
      {/* Filter toolbar */}
      <div className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--surface)] p-3">
        <div className="flex flex-wrap items-end gap-3">
          <div className="relative min-w-[200px] flex-1">
            <Search size={ICON.md} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]" aria-hidden="true" />
            <input
              value={qInput}
              onChange={(e) => setQInput(e.target.value)}
              placeholder={t("admin.logs.searchPlaceholder")}
              aria-label={t("admin.logs.searchPlaceholder")}
              className="w-full pl-9 pr-3 py-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
            />
          </div>
          <div className="w-[170px]">
            <Select label={t("admin.logs.where.label")} options={whereOptions} value={where} onChange={setWhere} />
          </div>
          <div className="w-[150px]">
            <Select label={t("admin.logs.source.label")} options={sourceOptions} value={source} onChange={setSource} />
          </div>
          <button
            type="button"
            onClick={() => setLive((v) => !v)}
            className="flex items-center gap-1.5 px-3 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast border"
            style={{
              backgroundColor: live ? "var(--accent)" : "var(--surface-raised)",
              color: live ? "var(--filter-active-text, #fff)" : "var(--text-muted)",
              borderColor: live ? "var(--accent)" : "var(--border)",
            }}
            aria-pressed={live}
          >
            <Radio size={ICON.sm} className={live ? "animate-pulse" : ""} aria-hidden="true" />
            {t("admin.logs.live")}
          </button>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-medium text-[var(--text-muted)]">{t("admin.logs.level.label")}</span>
          {LEVELS.map((l) => (
            <FilterChip key={l} active={levels.includes(l)} onClick={() => toggleLevel(l)}>
              {l}
            </FilterChip>
          ))}
          <FilterChip active={showAccess} onClick={() => setShowAccess((v) => !v)}>
            {t("admin.logs.accessLog")}
          </FilterChip>
          <button
            type="button"
            onClick={() => setShowAdvanced((v) => !v)}
            className="ml-auto flex items-center gap-1.5 text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast"
            aria-expanded={showAdvanced}
          >
            <SlidersHorizontal size={13} aria-hidden="true" />
            {t("admin.logs.advanced")}
          </button>
        </div>

        {showAdvanced && (
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-1">
            <Input label={t("admin.logs.sessionId")} value={sessionId} onChange={(e) => setSessionId(e.target.value)} placeholder="ps-..." />
            <Input label={t("admin.logs.userId")} value={userId} onChange={(e) => setUserId(e.target.value)} placeholder="alice, usr-..." />
            <Input label={t("admin.logs.mediaId")} value={mediaId} onChange={(e) => setMediaId(e.target.value)} placeholder="mi-..." />
          </div>
        )}
      </div>

      {/* Result meta */}
      <div className="flex flex-wrap items-center gap-3 text-sm text-[var(--text-muted)]">
        <span>{live ? t("admin.logs.liveCount", { count: total }) : t("admin.logs.resultCount", { count: total })}</span>
        {truncated && (
          <span className="flex items-center gap-1 text-[var(--badge-warning-text,#fde047)]">
            <AlertTriangle size={13} aria-hidden="true" />
            {t("admin.logs.truncated")}
          </span>
        )}
      </div>

      {/* Viewer */}
      <div className="rounded-[var(--radius-lg)] border border-[var(--border)] overflow-hidden">
        {isLoading && !live ? (
          <div className="flex justify-center py-16">
            <Spinner size="lg" />
          </div>
        ) : isError && !live ? (
          <EmptyState icon={<AlertTriangle size={ICON["3xl"]} strokeWidth={1.5} />} title={t("admin.logs.error")} />
        ) : entries.length === 0 ? (
          <EmptyState icon={<Search size={ICON["3xl"]} strokeWidth={1.5} />} title={t("admin.logs.empty")} />
        ) : (
          <div ref={parentRef} onScroll={captureAnchor} data-testid="log-viewport" className="relative max-h-[70vh] overflow-auto">
            {hiddenAbove > 0 && (
              <div className="sticky top-2 z-10 flex h-0 justify-center overflow-visible">
                <button
                  type="button"
                  onClick={jumpToTop}
                  className="flex items-center gap-1.5 rounded-full border border-[var(--accent)] bg-[var(--surface)] px-3 py-1 text-xs font-medium text-[var(--accent)] shadow transition-colors-fast hover:bg-[var(--surface-raised)]"
                >
                  <ArrowUp size={ICON.xs} aria-hidden="true" />
                  {t("admin.logs.newLines", { count: hiddenAbove })}
                </button>
              </div>
            )}
            <div style={{ height: virtualizer.getTotalSize(), width: "100%", position: "relative" }}>
              {virtualItems.map((vItem) => {
                const entry = entries[vItem.index];
                return (
                  <div
                    key={vItem.key}
                    data-index={vItem.index}
                    ref={virtualizer.measureElement}
                    style={{ position: "absolute", top: 0, left: 0, width: "100%", transform: `translateY(${vItem.start}px)` }}
                  >
                    <LogRow entry={entry} who={who} media={mediaLabels} open={openUids.has(entry.uid)} onToggle={toggleOpen} />
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

      {!live && isFetchingNextPage && (
        <div className="flex justify-center py-4">
          <Spinner size="md" />
        </div>
      )}
    </div>
  );
}
