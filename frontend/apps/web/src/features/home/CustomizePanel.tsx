import { useState, useEffect, useRef, useCallback, type PointerEvent as ReactPointerEvent } from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useFeedServiceGetFeedConfig,
  useFeedServiceUpdateProfileFeedConfig,
  useFeedServiceResetProfileFeedConfig,
  useLibraryServiceListLibraries,
  eveyFetch,
} from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { X, GripVertical, RotateCcw, ChevronUp, ChevronDown } from "lucide-react";
import { FEED_LABEL_KEYS, isKnownFeedType, isPluginFeedType, pluginIdFromFeedType, pluginFeedType } from "./feedTypes";
import { Switch } from "@/components/ui/Switch";
import { useFocusTrap } from "@/hooks/useFocusTrap";
import { ICON } from "@/constants/iconSizes";
import { useDemoMode } from "@/hooks/useDemoMode";
import { useDemoFeedConfig } from "./demoFeedConfig";

interface HomePlugin {
  id: string;
  name: string;
}

interface PluginListEntry {
  id?: string;
  name?: string;
  frontend?: { slot?: string }[];
}

function feedLabel(t: (k: string) => string, feedType: string, libraryNames: Map<string, string>, pluginNames: Map<string, string>): string {
  if (feedType.startsWith("library:")) {
    const libId = feedType.slice("library:".length);
    return libraryNames.get(libId) ?? feedType;
  }
  if (isPluginFeedType(feedType)) {
    const pid = pluginIdFromFeedType(feedType);
    const raw = pluginNames.get(pid) ?? `Plugin ${pid}`;
    // Plugin manifests don't yet ship a separate display name - title-case
    // the raw `name` field so identifiers like "weather" don't sit next to
    // properly formatted shelf labels.
    return raw.charAt(0).toUpperCase() + raw.slice(1);
  }
  return isKnownFeedType(feedType) ? t(FEED_LABEL_KEYS[feedType]) : feedType;
}

interface Row {
  feedType: string;
  enabled: boolean;
  displayOrder: number;
}

export function CustomizePanel({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId);
  const queryClient = useQueryClient();
  const panelRef = useRef<HTMLDivElement>(null);
  // aria-modal claims focus is trapped; actually trap it so Tab can't escape to
  // the page behind the slide-over while it is open.
  useFocusTrap(panelRef, open);
  const { data } = useFeedServiceGetFeedConfig({ profileId: profileId ?? undefined });
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraryNames = new Map(
    (librariesData?.libraries ?? []).map((l) => [l.id!, l.name ?? l.id!]),
  );

  // Reuse the same query key as PluginSlot so installing/uninstalling a
  // plugin invalidates this list too - no stale plugin entries in the
  // customise menu after the user changes their plugin set.
  const { data: pluginsData } = useQuery<{ plugins?: PluginListEntry[] }>({
    queryKey: ["/api/v1/plugins"],
    queryFn: () => eveyFetch<{ plugins?: PluginListEntry[] }>({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
  });
  // Only plugins that actually render on the home page (slot
  // "home.section") are eligible for the customise menu. Anything else
  // would be a fake row the user couldn't see flip on/off.
  const homePlugins: HomePlugin[] = (pluginsData?.plugins ?? [])
    .filter((p): p is PluginListEntry & { id: string } => Boolean(p.id))
    .filter((p) => (p.frontend ?? []).some((m) => m.slot === "home.section"))
    .map((p) => ({ id: p.id, name: p.name ?? p.id }));
  const pluginNames = new Map(homePlugins.map((p) => [p.id, p.name]));
  const updateMutation = useFeedServiceUpdateProfileFeedConfig();
  const resetMutation = useFeedServiceResetProfileFeedConfig();
  // Demo: the profile is shared, so the layout is this browser's (see
  // demoFeedConfig) and nothing is sent to the server, which would refuse it.
  const demo = useDemoMode();
  const [demoRows, writeDemoRows] = useDemoFeedConfig();

  const [rows, setRows] = useState<Row[]>([]);
  const [dragIdx, setDragIdx] = useState<number | null>(null);
  const [overIdx, setOverIdx] = useState<number | null>(null);
  const dragIdxRef = useRef<number | null>(null);
  const overIdxRef = useRef<number | null>(null);

  useEffect(() => {
    const source = demo && demoRows ? demoRows : data?.configs;
    if (!source) return;
    const installedPluginIds = new Set(homePlugins.map((p) => p.id));
    const configured = source
      .map((c) => ({
        feedType: c.feedType ?? "",
        enabled: c.enabled ?? true,
        displayOrder: c.displayOrder ?? 0,
      }))
      // Drop feed types that have no renderable section on the home page
      // (e.g. legacy `continue_watching`, `recently_added`, `next_episode`).
      // Keep `library:*`, `plugin:*` and the canonical KNOWN_FEED_TYPES.
      .filter((c) => isKnownFeedType(c.feedType) || c.feedType.startsWith("library:") || isPluginFeedType(c.feedType))
      // Drop orphan plugin rows: configs persisted for a plugin id that is
      // no longer installed (uninstalled, or reinstalled under a new id).
      // Without this filter the user sees both the live plugin label AND a
      // ghost "Plugin <stale-id>" row pointing at nothing.
      .filter((c) => !isPluginFeedType(c.feedType) || installedPluginIds.has(pluginIdFromFeedType(c.feedType)));

    // Prepend home-section plugins that aren't in the user's config yet
    // (newly installed). They impact the page layout, so default to the
    // top - the user can still drag them down later. Negative orders are
    // safe: setRows then sorts ascending and the save handler renormalises
    // to contiguous integers before persisting.
    const haveAlready = new Set(configured.map((r) => r.feedType));
    const minOrder = configured.reduce((m, r) => Math.min(m, r.displayOrder), 0);
    const newPlugins = homePlugins
      .filter((p) => !haveAlready.has(pluginFeedType(p.id)))
      .map((p, i) => ({
        feedType: pluginFeedType(p.id),
        enabled: true,
        displayOrder: minOrder - homePlugins.length + i,
      }));

    setRows([...configured, ...newPlugins].sort((a, b) => a.displayOrder - b.displayOrder));
    // Intentionally exclude homePlugins from deps to avoid the array
    // identity churn from the useQuery refetch - it only matters when the
    // set actually changes, which we already pick up via pluginsData.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, pluginsData, demo, demoRows]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open, onClose]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [open, onClose]);

  const toggle = (i: number) => {
    const snapshot = rows;
    setRows((prev) => {
      const next = [...prev];
      next[i] = { ...next[i], enabled: !next[i].enabled, displayOrder: i };
      return next;
    });
    const row = rows[i];
    if (demo) {
      writeDemoRows(rows.map((r, j) => ({ feedType: r.feedType, enabled: j === i ? !r.enabled : r.enabled, displayOrder: j })));
      return;
    }
    // Persist using the rendered index `i` rather than row.displayOrder so
    // freshly-added plugin rows (which start at a negative order to land at
    // the top) get a clean 0..N-1 value in the DB the first time the user
    // interacts with them.
    updateMutation.mutate(
      { feedType: row.feedType, data: { enabled: !row.enabled, displayOrder: i } },
      {
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] }),
        onError: () => { setRows(snapshot); toast.error(t("common.error")); },
      },
    );
  };

  const commitReorder = useCallback((from: number, to: number) => {
    if (from === to) return;
    const snapshot = rows;
    setRows((prev) => {
      const next = [...prev];
      const [moved] = next.splice(from, 1);
      next.splice(to, 0, moved);
      return next.map((r, i) => ({ ...r, displayOrder: i }));
    });
    // Persist both swapped items; on the first network error restore the
    // snapshot and toast once (errored flag guards against duplicate toasts
    // when several mutations fail for the same drag).
    const reordered = [...rows];
    const [moved] = reordered.splice(from, 1);
    reordered.splice(to, 0, moved);
    if (demo) {
      writeDemoRows(reordered.map((r, i) => ({ feedType: r.feedType, enabled: r.enabled, displayOrder: i })));
      return;
    }
    let errored = false;
    reordered.forEach((r, i) => {
      if (r.displayOrder !== i) {
        updateMutation.mutate(
          { feedType: r.feedType, data: { enabled: r.enabled, displayOrder: i } },
          {
            onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] }),
            onError: () => {
              if (!errored) {
                errored = true;
                setRows(snapshot);
                toast.error(t("common.error"));
              }
            },
          },
        );
      }
    });
  }, [rows, updateMutation, queryClient, t, demo, writeDemoRows]);

  const startPointerDrag = useCallback((i: number, e: ReactPointerEvent) => {
    e.preventDefault();
    dragIdxRef.current = i;
    overIdxRef.current = i;
    setDragIdx(i);
    setOverIdx(i);
  }, []);

  // Pointer-based reordering: works on touch AND mouse. The previous native
  // HTML5 drag&drop (draggable/onDragStart) has NO touch support, so the drag
  // handle did nothing on phones/tablets.
  useEffect(() => {
    if (dragIdx === null) return;
    const onMove = (e: PointerEvent) => {
      const el = document.elementFromPoint(e.clientX, e.clientY);
      const row = el?.closest("[data-dragidx]");
      if (row) {
        const j = Number(row.getAttribute("data-dragidx"));
        if (!Number.isNaN(j)) { overIdxRef.current = j; setOverIdx(j); }
      }
    };
    const onUp = () => {
      const from = dragIdxRef.current;
      const to = overIdxRef.current;
      if (from !== null && to !== null && from !== to) commitReorder(from, to);
      dragIdxRef.current = null;
      overIdxRef.current = null;
      setDragIdx(null);
      setOverIdx(null);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onUp);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onUp);
    };
  }, [dragIdx, commitReorder]);

  const resetAll = () => {
    if (demo) {
      // Back to the shared layout: the local one is simply forgotten.
      writeDemoRows(null);
      return;
    }
    rows.forEach((r) => {
      resetMutation.mutate(
        { feedType: r.feedType },
        { onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] }) },
      );
    });
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-overlay flex justify-end bg-[var(--overlay-chip-bg)]">
      <div
        ref={panelRef}
        role="dialog"
        aria-modal={true}
        aria-label={t("common.feeds.customizeTitle")}
        className="w-80 h-full overflow-y-auto animate-fade-in bg-[var(--surface)] border-l border-[var(--border)]"
        data-testid="customize-panel"
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
          <h2 className="text-base font-semibold text-[var(--text)]">
            {t("common.feeds.customizeTitle")}
          </h2>
          <button onClick={onClose} className="p-1 rounded hover:opacity-80 text-[var(--text-muted)]" aria-label={t("common.aria.close")}>
            <X size={ICON.lg}  aria-hidden="true" />
          </button>
        </div>

        <div className="p-4 space-y-1">
          {rows.map((row, i) => (
            <div
              key={row.feedType}
              data-dragidx={i}
              className="flex items-center gap-2 px-3 py-2.5 rounded-[var(--radius-md)] transition-all"
              style={{
                backgroundColor: overIdx === i && dragIdx !== null && dragIdx !== i ? "var(--accent-subtle)" : "var(--bg)",
                opacity: dragIdx === i ? 0.5 : 1,
              }}
            >
              <GripVertical
                size={14}
                onPointerDown={(e) => startPointerDrag(i, e)}
                style={{ touchAction: "none" }}
                aria-hidden="true"
                className="flex-shrink-0 cursor-grab active:cursor-grabbing text-[var(--text-muted)]"
              />

              {/* Keyboard path to the same reorder. The grip is pointer-only by
                  nature (pointerdown + pointermove), so without these a keyboard
                  user could not move a section at all - the catalogue even
                  carried common.feeds.moveUp/moveDown, unused, from when this
                  was planned. Same shape as the SyncPlay queue's reorder
                  buttons. */}
              <button
                type="button"
                onClick={() => commitReorder(i, i - 1)}
                disabled={i === 0}
                aria-label={t("common.feeds.moveUp")}
                title={t("common.feeds.moveUp")}
                className="p-0.5 text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-30 cursor-pointer disabled:cursor-default"
              >
                <ChevronUp size={14} />
              </button>
              <button
                type="button"
                onClick={() => commitReorder(i, i + 1)}
                disabled={i === rows.length - 1}
                aria-label={t("common.feeds.moveDown")}
                title={t("common.feeds.moveDown")}
                className="p-0.5 text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-30 cursor-pointer disabled:cursor-default"
              >
                <ChevronDown size={14} />
              </button>

              <span
                className="flex-1 text-sm select-none"
                style={{ color: row.enabled ? "var(--text)" : "var(--text-muted)" }}
              >
                {feedLabel(t, row.feedType, libraryNames, pluginNames)}
              </span>

              <Switch
                checked={row.enabled}
                onChange={() => toggle(i)}
                label={feedLabel(t, row.feedType, libraryNames, pluginNames)}
              />
            </div>
          ))}
        </div>

        <div className="px-4 pb-4">
          <button
            onClick={resetAll}
            className="flex items-center gap-2 text-sm hover:opacity-80 transition-opacity-fast text-[var(--text-muted)]"
            data-testid="customize-reset-btn"
          >
            <RotateCcw size={14} />
            {t("common.feeds.resetDefaults")}
          </button>
        </div>
      </div>
    </div>
  );
}
