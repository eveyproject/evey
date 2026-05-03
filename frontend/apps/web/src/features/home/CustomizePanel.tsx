import { useState, useEffect, useRef, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useFeedServiceGetFeedConfig,
  useFeedServiceUpdateProfileFeedConfig,
  useFeedServiceResetProfileFeedConfig,
  useLibraryServiceListLibraries,
  eveyFetch,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { X, GripVertical, RotateCcw } from "lucide-react";
import { FEED_LABEL_KEYS, isKnownFeedType, isPluginFeedType, pluginIdFromFeedType, pluginFeedType } from "./feedTypes";

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
    return pluginNames.get(pid) ?? `Plugin ${pid}`;
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
  const { data } = useFeedServiceGetFeedConfig({ profileId: profileId ?? undefined });
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraryNames = new Map(
    (librariesData?.libraries ?? []).map((l) => [l.id!, l.name ?? l.id!]),
  );

  // Reuse the same query key as PluginSlot so installing/uninstalling a
  // plugin invalidates this list too — no stale plugin entries in the
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

  const [rows, setRows] = useState<Row[]>([]);
  const [dragIdx, setDragIdx] = useState<number | null>(null);
  const [overIdx, setOverIdx] = useState<number | null>(null);

  useEffect(() => {
    if (!data?.configs) return;
    const configured = data.configs
      .map((c) => ({
        feedType: c.feedType ?? "",
        enabled: c.enabled ?? true,
        displayOrder: c.displayOrder ?? 0,
      }))
      // Drop feed types that have no renderable section on the home page
      // (e.g. legacy `continue_watching`, `recently_added`, `next_episode`).
      // Keep `library:*`, `plugin:*` and the canonical KNOWN_FEED_TYPES.
      .filter((c) => isKnownFeedType(c.feedType) || c.feedType.startsWith("library:") || isPluginFeedType(c.feedType));

    // Append home-section plugins that aren't in the user's config yet
    // (newly installed). Default enabled=true, displayOrder past the end
    // so they land at the bottom and the user can drag them up.
    const haveAlready = new Set(configured.map((r) => r.feedType));
    const maxOrder = configured.reduce((m, r) => Math.max(m, r.displayOrder), -1);
    const newPlugins = homePlugins
      .filter((p) => !haveAlready.has(pluginFeedType(p.id)))
      .map((p, i) => ({
        feedType: pluginFeedType(p.id),
        enabled: true,
        displayOrder: maxOrder + 1 + i,
      }));

    setRows([...configured, ...newPlugins].sort((a, b) => a.displayOrder - b.displayOrder));
    // Intentionally exclude homePlugins from deps to avoid the array
    // identity churn from the useQuery refetch — it only matters when the
    // set actually changes, which we already pick up via pluginsData.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, pluginsData]);

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
    setRows((prev) => {
      const next = [...prev];
      next[i] = { ...next[i], enabled: !next[i].enabled };
      return next;
    });
    const row = rows[i];
    updateMutation.mutate(
      { feedType: row.feedType, data: { enabled: !row.enabled, displayOrder: row.displayOrder } },
      { onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] }) },
    );
  };

  const commitReorder = useCallback((from: number, to: number) => {
    if (from === to) return;
    setRows((prev) => {
      const next = [...prev];
      const [moved] = next.splice(from, 1);
      next.splice(to, 0, moved);
      return next.map((r, i) => ({ ...r, displayOrder: i }));
    });
    // Persist both swapped items
    const reordered = [...rows];
    const [moved] = reordered.splice(from, 1);
    reordered.splice(to, 0, moved);
    reordered.forEach((r, i) => {
      if (r.displayOrder !== i) {
        updateMutation.mutate(
          { feedType: r.feedType, data: { enabled: r.enabled, displayOrder: i } },
          { onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] }) },
        );
      }
    });
  }, [rows, updateMutation, queryClient]);

  const handleDragEnd = useCallback(() => {
    if (dragIdx !== null && overIdx !== null && dragIdx !== overIdx) {
      commitReorder(dragIdx, overIdx);
    }
    setDragIdx(null);
    setOverIdx(null);
  }, [dragIdx, overIdx, commitReorder]);

  const resetAll = () => {
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
        className="w-80 h-full overflow-y-auto animate-fade-in bg-[var(--surface)] border-l border-[var(--border)]"
        data-testid="customize-panel"
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
          <h2 className="text-base font-semibold text-[var(--text)]">
            {t("common.feeds.customizeTitle")}
          </h2>
          <button onClick={onClose} className="p-1 rounded hover:opacity-80 text-[var(--text-muted)]" aria-label={t("common.aria.close")}>
            <X size={18}  aria-hidden="true" />
          </button>
        </div>

        <div className="p-4 space-y-1">
          {rows.map((row, i) => (
            <div
              key={row.feedType}
              draggable
              onDragStart={() => setDragIdx(i)}
              onDragOver={(e) => { e.preventDefault(); setOverIdx(i); }}
              onDragEnd={handleDragEnd}
              className="flex items-center gap-2 px-3 py-2.5 rounded-[var(--radius-md)] transition-all"
              style={{
                backgroundColor: overIdx === i && dragIdx !== null && dragIdx !== i ? "var(--accent-subtle)" : "var(--bg)",
                opacity: dragIdx === i ? 0.5 : 1,
              }}
            >
              <GripVertical size={14} className="flex-shrink-0 cursor-grab active:cursor-grabbing text-[var(--text-muted)]" />

              <span
                className="flex-1 text-sm select-none"
                style={{ color: row.enabled ? "var(--text)" : "var(--text-muted)" }}
              >
                {feedLabel(t, row.feedType, libraryNames, pluginNames)}
              </span>

              <button
                role="switch"
                aria-checked={row.enabled}
                aria-label={feedLabel(t, row.feedType, libraryNames, pluginNames)}
                onClick={() => toggle(i)}
                className="w-9 h-5 rounded-full transition-colors-fast relative flex-shrink-0"
                style={{ backgroundColor: row.enabled ? "var(--accent)" : "var(--surface-raised)" }}
              >
                <span
                  className="absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform-fast"
                  style={{ left: row.enabled ? "calc(100% - 1.125rem)" : "0.125rem" }}
                />
              </button>
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
