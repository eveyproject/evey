import { useState, useEffect, useCallback, useRef, type PointerEvent as ReactPointerEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  useFeedServiceGetAdminFeedConfigs,
  useLibraryServiceListLibraries,
  useAdminPluginServiceListPlugins,
  feedServiceBatchUpdateAdminFeedConfigs,
  getFeedServiceGetAdminFeedConfigsQueryKey,
  getFeedServiceGetFeedConfigQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { GripVertical, ChevronUp, ChevronDown, Puzzle } from "lucide-react";
import { Switch } from "@/components/ui/Switch";

interface FeedRow {
  feedType: string;
  enabled: boolean;
  displayOrder: number;
  maxItems: number;
}

const FEED_LABEL_KEYS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  recommended: "common.feeds.labels.recommended",
  most_popular: "common.feeds.labels.most_popular",
  collections: "common.feeds.labels.collections",
};

// Header and rows share one template so every column lines up.
const ROW_GRID = "grid grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-3 sm:gap-4";

function feedLabel(
  t: (k: string) => string,
  feedType: string,
  libraryNames: Map<string, string>,
): string {
  if (feedType.startsWith("library:")) {
    const libId = feedType.slice("library:".length);
    return libraryNames.get(libId) ?? feedType;
  }
  return FEED_LABEL_KEYS[feedType] ? t(FEED_LABEL_KEYS[feedType]) : feedType;
}

// A plugin row is a plugin's home.section. The admin decides whether it shows
// and where it sits by default; a user who reorders their own home in the
// Customize panel still wins over the admin order, exactly as for built-in rows.
// Its size is the plugin's business, so it has no max items field.
function isPluginRow(feedType: string): boolean {
  return feedType.startsWith("plugin:");
}

function pluginLabel(
  feedType: string,
  pluginNames: Map<string, string>,
): string {
  const id = feedType.slice("plugin:".length);
  return pluginNames.get(id) ?? id;
}

function move<T>(list: T[], from: number, to: number): T[] {
  const next = [...list];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved);
  return next;
}

export function FeedConfigCard() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data, isLoading } = useFeedServiceGetAdminFeedConfigs();
  const { data: librariesData } = useLibraryServiceListLibraries();
  const { data: pluginsData } = useAdminPluginServiceListPlugins();
  const libraryNames = new Map(
    (librariesData?.libraries ?? []).map((l) => [l.id!, l.name ?? l.id!]),
  );
  // Only plugins that expose a `home.section` frontend slot belong on the home
  // page; the others (admin.nav and friends) have nothing to show there.
  const homePlugins = (pluginsData?.plugins ?? []).filter(
    (p) =>
      p.enabled &&
      (p.frontend ?? []).some((mod) => mod.slot === "home.section"),
  );
  const pluginNames = new Map(
    homePlugins.map((p) => [p.id ?? "", p.name ?? p.id ?? ""]),
  );
  const [rows, setRows] = useState<FeedRow[]>([]);
  const [saving, setSaving] = useState(false);
  // The dragged row is tracked by feed type, not index: the list reorders
  // live under the pointer, so an index would point at another row mid-drag.
  const [dragKey, setDragKey] = useState<string | null>(null);
  // dirty tracks unsaved local edits. While true, background refetches must
  // NOT overwrite in-progress changes.
  const [dirty, setDirty] = useState(false);
  const listRef = useRef<HTMLDivElement>(null);
  const rowsRef = useRef(rows);
  rowsRef.current = rows;

  useEffect(() => {
    if (dirty) return;
    if (data?.configs) {
      const persisted = data.configs.map((c) => ({
        feedType: c.feedType ?? "",
        enabled: c.enabled ?? true,
        displayOrder: c.displayOrder ?? 0,
        maxItems: c.maxItems ?? 20,
      }));
      // Plugins without an admin row yet are new: they go on top, the last
      // installed first, which is where every client's home already renders
      // a plugin the config does not place.
      const persistedTypes = new Set(persisted.map((r) => r.feedType));
      const virtualPluginRows = homePlugins
        .filter((p) => !persistedTypes.has(`plugin:${p.id ?? ""}`))
        .sort((a, b) => (Date.parse(b.installedAt ?? "") || 0) - (Date.parse(a.installedAt ?? "") || 0))
        .map((p) => ({
          feedType: `plugin:${p.id ?? ""}`,
          enabled: true,
          displayOrder: 0,
          maxItems: 20,
        }));
      setRows(
        [
          ...virtualPluginRows,
          ...persisted.sort((a, b) => a.displayOrder - b.displayOrder),
        ].map((r, i) => ({ ...r, displayOrder: i })),
      );
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, pluginsData, dirty]);

  const commitReorder = useCallback((from: number, to: number) => {
    setRows((prev) => {
      if (from === to || to < 0 || to >= prev.length) return prev;
      return move(prev, from, to).map((r, i) => ({ ...r, displayOrder: i }));
    });
    setDirty(true);
  }, []);

  const startPointerDrag = useCallback((feedType: string, e: ReactPointerEvent) => {
    e.preventDefault();
    setDragKey(feedType);
  }, []);

  // Pointer-based reordering works on touch AND mouse (native HTML5 drag and
  // drop has no touch support). The row follows the pointer: its slot is the
  // number of other rows whose vertical middle sits above the pointer.
  useEffect(() => {
    if (dragKey === null) return;
    const onMove = (e: PointerEvent) => {
      const list = listRef.current;
      if (!list) return;
      const els = Array.from(list.querySelectorAll<HTMLElement>("[data-feedrow]"));
      let target = 0;
      for (const el of els) {
        if (el.dataset.feedrow === dragKey) continue;
        const r = el.getBoundingClientRect();
        if (e.clientY > r.top + r.height / 2) target++;
      }
      const from = rowsRef.current.findIndex((r) => r.feedType === dragKey);
      if (from < 0 || from === target) return;
      commitReorder(from, target);
    };
    const onUp = () => setDragKey(null);
    const previousCursor = document.body.style.cursor;
    document.body.style.cursor = "grabbing";
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onUp);
    return () => {
      document.body.style.cursor = previousCursor;
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onUp);
    };
  }, [dragKey, commitReorder]);

  const toggle = (i: number) => {
    setRows((prev) =>
      prev.map((r, idx) => (idx === i ? { ...r, enabled: !r.enabled } : r)),
    );
    setDirty(true);
  };

  const setMaxItems = (i: number, v: number) => {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, maxItems: v } : r)));
    setDirty(true);
  };

  // Clearing dirty lets the sync effect rebuild the rows from the server copy.
  const discard = () => setDirty(false);

  const save = async () => {
    setSaving(true);
    try {
      await feedServiceBatchUpdateAdminFeedConfigs({ configs: rows });
      toast.success(t("common.feeds.saved"));
      // Await the refetch of THIS card's source query before clearing dirty:
      // the rows-sync effect is gated on !dirty, so clearing it while the cache
      // still holds the pre-save config would momentarily rebuild rows from the
      // stale data and flash the old order/maxItems until the refetch lands.
      await queryClient.invalidateQueries({
        queryKey: getFeedServiceGetAdminFeedConfigsQueryKey(),
      });
      setDirty(false);
      // The home page reads merged config via GetFeedConfig (admin defaults
      // + profile overrides). Without this second invalidation the admin
      // save lands in DB but every open HP keeps showing the stale merged
      // result until the 60s staleTime expires.
      queryClient.invalidateQueries({
        queryKey: getFeedServiceGetFeedConfigQueryKey(),
      });
    } catch {
      toast.error(t("common.error"));
    } finally {
      setSaving(false);
    }
  };

  if (isLoading) {
    return <div className="animate-shimmer h-40 rounded-[var(--radius-md)]" />;
  }

  const enabledCount = rows.filter((r) => r.enabled).length;
  const iconBtn =
    "flex items-center justify-center w-6 h-5 rounded-[var(--radius-sm)] text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-hover)] disabled:opacity-25 disabled:hover:bg-transparent cursor-pointer disabled:cursor-default focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--accent)]";

  return (
    <div className="max-w-2xl rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--surface)]">
      <p className="px-4 pt-3 pb-2 text-xs text-[var(--text-muted)]">
        {t("admin.serverConfig.feedConfig.hint")}
      </p>

      <div
        className={`${ROW_GRID} px-4 py-2 text-[11px] font-semibold uppercase tracking-wider text-[var(--text-muted)] border-y border-[var(--border)]`}
      >
        <span className="w-[4.5rem]" />
        <span>{t("common.feeds.section")}</span>
        <span className="w-20 text-center">{t("common.feeds.maxItems")}</span>
        <span className="w-12 text-center">{t("common.enabled")}</span>
      </div>

      <div ref={listRef} className={dragKey ? "select-none" : undefined}>
        {rows.map((row, i) => {
          const plugin = isPluginRow(row.feedType);
          const library = row.feedType.startsWith("library:");
          const label = plugin
            ? pluginLabel(row.feedType, pluginNames)
            : feedLabel(t, row.feedType, libraryNames);
          const dragging = dragKey === row.feedType;
          const kind = plugin
            ? null
            : library
              ? t("admin.serverConfig.feedConfig.kindLibrary")
              : t("admin.serverConfig.feedConfig.kindBuiltin");
          return (
            <div
              key={row.feedType}
              data-feedrow={row.feedType}
              className={`group ${ROW_GRID} relative px-4 py-2.5 border-b border-[var(--border)] last:border-b-0 transition-[background-color,box-shadow] duration-150 ${
                dragging
                  ? "z-10 bg-[var(--surface-raised)] shadow-[var(--shadow-lg)] ring-1 ring-[var(--accent)] rounded-[var(--radius-md)]"
                  : "hover:bg-[var(--surface-hover)]"
              }`}
              style={
                plugin && !dragging
                  ? {
                      backgroundColor: "color-mix(in srgb, var(--accent) 6%, transparent)",
                      boxShadow: "inset 3px 0 0 color-mix(in srgb, var(--accent) 70%, transparent)",
                    }
                  : undefined
              }
            >
              <div className="flex items-center gap-1.5 w-[4.5rem]">
                <span
                  onPointerDown={(e) => startPointerDrag(row.feedType, e)}
                  style={{ touchAction: "none" }}
                  className="flex items-center justify-center w-6 h-8 rounded-[var(--radius-sm)] cursor-grab active:cursor-grabbing text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-raised)]"
                  aria-hidden="true"
                >
                  <GripVertical size={16} />
                </span>
                {/* The grip only answers a pointer: these two buttons are the
                    keyboard and screen reader path to the same reorder. */}
                <div className="flex flex-col">
                  <button
                    type="button"
                    onClick={() => commitReorder(i, i - 1)}
                    disabled={i === 0}
                    aria-label={t("common.feeds.moveUp")}
                    title={t("common.feeds.moveUp")}
                    className={iconBtn}
                  >
                    <ChevronUp size={14} />
                  </button>
                  <button
                    type="button"
                    onClick={() => commitReorder(i, i + 1)}
                    disabled={i === rows.length - 1}
                    aria-label={t("common.feeds.moveDown")}
                    title={t("common.feeds.moveDown")}
                    className={iconBtn}
                  >
                    <ChevronDown size={14} />
                  </button>
                </div>
                <span className="w-5 text-right text-xs tabular-nums text-[var(--text-muted)]">
                  {i + 1}
                </span>
              </div>

              <div
                className="min-w-0 transition-opacity duration-150"
                style={{ opacity: row.enabled ? 1 : 0.5 }}
              >
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-sm font-medium truncate text-[var(--text)]" title={label}>
                    {label}
                  </span>
                  {plugin && (
                    <span className="shrink-0 inline-flex items-center gap-1 text-[10px] font-semibold uppercase tracking-wider px-1.5 py-0.5 rounded-[var(--radius-sm)] bg-[var(--accent-subtle)] text-[var(--accent)]">
                      <Puzzle size={10} aria-hidden="true" />
                      {t("admin.serverConfig.feedConfig.pluginBadge", "Plugin")}
                    </span>
                  )}
                </div>
                {kind && (
                  <span className="block text-xs text-[var(--text-muted)] truncate">{kind}</span>
                )}
              </div>

              <div className="w-20 flex justify-center">
                {plugin ? (
                  <span
                    className="text-xs text-[var(--text-muted)]"
                    title={t("admin.serverConfig.feedConfig.pluginMaxItems")}
                  >
                    -
                  </span>
                ) : (
                  <input
                    type="number"
                    min={1}
                    inputMode="numeric"
                    aria-label={`${t("common.feeds.maxItems")}: ${label}`}
                    value={row.maxItems}
                    disabled={!row.enabled}
                    onChange={(e) => {
                      // Guard NaN (empty / non-numeric paste): keep the old value
                      // rather than persisting NaN to the backend on Save.
                      const v = Number(e.target.value);
                      setMaxItems(i, Number.isFinite(v) ? Math.max(1, v) : row.maxItems);
                    }}
                    className="w-16 h-8 text-center text-sm tabular-nums rounded-[var(--radius-sm)] border bg-[var(--bg)] border-[var(--border)] text-[var(--text)] focus:outline-none focus:border-[var(--accent)] disabled:opacity-40"
                  />
                )}
              </div>

              <div className="w-12 flex justify-center">
                <Switch
                  checked={row.enabled}
                  onChange={() => toggle(i)}
                  label={`${row.enabled ? t("common.disable") : t("common.enable")}: ${label}`}
                />
              </div>
            </div>
          );
        })}
      </div>

      <div className="sticky bottom-0 z-20 flex flex-wrap items-center justify-between gap-3 px-4 py-3 border-t border-[var(--border)] bg-[var(--surface)] rounded-b-[var(--radius-lg)]">
        <span className="text-xs text-[var(--text-muted)]" data-testid="feeds-status">
          {dirty ? (
            <span className="inline-flex items-center gap-2 text-[var(--text)]">
              <span className="w-2 h-2 rounded-full bg-[var(--accent)]" aria-hidden="true" />
              {t("admin.serverConfig.feedConfig.unsaved")}
            </span>
          ) : (
            t("admin.serverConfig.feedConfig.summary", { enabled: enabledCount, total: rows.length })
          )}
        </span>
        <div className="flex items-center gap-2">
          {dirty && (
            <button
              type="button"
              onClick={discard}
              disabled={saving}
              className="px-3 py-1.5 rounded-[var(--radius-md)] text-sm text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-hover)] disabled:opacity-50"
              data-testid="feeds-discard-btn"
            >
              {t("common.cancel")}
            </button>
          )}
          <button
            type="button"
            onClick={save}
            disabled={saving || !dirty}
            className="px-4 py-1.5 rounded-[var(--radius-md)] text-sm font-medium transition-opacity-fast hover:opacity-90 disabled:opacity-40 disabled:cursor-default bg-[var(--accent)] text-[var(--filter-active-text)]"
            aria-label={t("common.save")}
            data-testid="feeds-save-btn"
          >
            {saving ? t("common.loading") : t("common.save")}
          </button>
        </div>
      </div>
    </div>
  );
}
