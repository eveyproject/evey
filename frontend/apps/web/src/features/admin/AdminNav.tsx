import { useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  LayoutDashboard,
  PlayCircle,
  Globe,
  Library,
  HardDrive,
  Users,
  Puzzle,
  Webhook,
  Key,
  BarChart3,
  TrendingUp,
  Gauge,
  Database,
  Cpu,
  SlidersHorizontal,
  Film,
  ChevronDown,
  Image,
  ScanSearch,
  ScrollText,
  Inbox,
  Share2,
  type LucideIcon,
} from "lucide-react";
import { useDemoStore } from "@evey/store";
import { usePluginSlotEntries } from "@/features/plugins/usePluginSlot";
import { useDemoMode } from "@/hooks/useDemoMode";
import { ICON } from "@/constants/iconSizes";

interface NavLink {
  href: string;
  labelKey: string;
  icon: LucideIcon;
  // The route the page reads, when the demos refuse it outright: the entry is
  // left out there (everything on the public demo, the listed keys on the
  // extended one - the same rule as useDemoLockFor) rather than leading to a
  // page that can only say no.
  demoLockKey?: string;
}

interface NavSection {
  titleKey: string;
  links: NavLink[];
}

const sectionsDef: NavSection[] = [
  {
    titleKey: "admin.nav.dashboardSystem",
    links: [
      { href: "/admin", labelKey: "admin.nav.dashboard", icon: LayoutDashboard },
      { href: "/admin/sessions", labelKey: "admin.nav.nowPlaying", icon: PlayCircle },
      { href: "/admin/remote-access", labelKey: "admin.nav.remoteAccess", icon: Globe },
      { href: "/admin/server-config", labelKey: "admin.nav.serverConfig", icon: SlidersHorizontal },
      { href: "/admin/transcoding", labelKey: "admin.nav.transcodingConfig", icon: Film },
      { href: "/admin/system-health", labelKey: "admin.nav.systemHealth", icon: Cpu },
    ],
  },
  {
    titleKey: "admin.nav.media",
    links: [
      { href: "/admin/libraries", labelKey: "admin.nav.libraries", icon: Library },
      { href: "/admin/transcode-cache", labelKey: "admin.nav.transcodeCache", icon: HardDrive },
      { href: "/admin/artworks", labelKey: "admin.nav.artworks", icon: Image },
      { href: "/admin/media-health", labelKey: "admin.nav.seekIndex", icon: ScanSearch },
    ],
  },
  {
    titleKey: "admin.nav.usersAccess",
    links: [
      { href: "/admin/users", labelKey: "admin.nav.users", icon: Users },
      { href: "/admin/media-requests", labelKey: "admin.nav.mediaRequests", icon: Inbox },
      { href: "/admin/share-links", labelKey: "admin.nav.shareLinks", icon: Share2 },
    ],
  },
  {
    titleKey: "admin.nav.integrations",
    links: [
      { href: "/admin/plugins", labelKey: "admin.nav.plugins", icon: Puzzle },
      { href: "/admin/webhooks", labelKey: "admin.nav.webhooks", icon: Webhook },
      { href: "/admin/api-keys", labelKey: "admin.nav.apiKeys", icon: Key },
    ],
  },
  {
    titleKey: "admin.nav.monitoring",
    links: [
      { href: "/admin/analytics", labelKey: "admin.nav.analytics", icon: TrendingUp },
      { href: "/admin/activity", labelKey: "admin.nav.activity", icon: BarChart3 },
      { href: "/admin/logs", labelKey: "admin.nav.logs", icon: ScrollText, demoLockKey: "GET /api/v1/admin/logs" },
      { href: "/admin/qoe", labelKey: "admin.nav.qoe", icon: Gauge },
    ],
  },
  {
    titleKey: "admin.nav.maintenance",
    links: [
      { href: "/admin/backup", labelKey: "admin.nav.backupRestore", icon: Database },
    ],
  },
];

function isLinkActive(pathname: string, href: string): boolean {
  if (href === "/admin") return pathname === "/admin" || pathname === "/admin/";
  return pathname === href || pathname.startsWith(href + "/");
}

export interface AdminNavProps {
  collapsed?: boolean;
  onNavigate?: () => void;
}

export function AdminNav({ collapsed = false, onNavigate }: AdminNavProps) {
  const { t } = useTranslation();
  const { location } = useRouterState();
  const pathname = location.pathname;
  const pluginEntries = usePluginSlotEntries("admin.nav");
  const demo = useDemoMode();
  const demoLocked = useDemoStore((s) => s.demoLocked);
  const offered = (link: NavLink) =>
    link.demoLockKey === undefined || !(demo || demoLocked.includes(link.demoLockKey));

  const [sectionCollapsed, setSectionCollapsed] = useState<Record<string, boolean>>({});

  function toggleSection(title: string) {
    setSectionCollapsed((prev) => ({ ...prev, [title]: !prev[title] }));
  }

  return (
    <div className="flex flex-col gap-1">
      {sectionsDef.map((section, idx) => {
        const sectionTitle = t(section.titleKey);
        const isCollapsed = sectionCollapsed[section.titleKey] ?? false;

        return (
          <div key={section.titleKey} className={idx > 0 ? "mt-2" : ""}>
            {!collapsed && (
              <button
                type="button"
                onClick={() => toggleSection(section.titleKey)}
                className="w-full flex items-center justify-between px-2 py-1.5 cursor-pointer rounded-[var(--radius-sm)] transition-colors-fast hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
                aria-expanded={!isCollapsed}
                aria-label={sectionTitle}
              >
                <span
                  className="text-xs font-semibold uppercase tracking-wider select-none"
                >
                  {sectionTitle}
                </span>
                <ChevronDown
                  size={ICON.xs}
                  className="transition-transform-fast"
                  style={{
                    transform: isCollapsed ? "rotate(-90deg)" : "rotate(0deg)",
                  }}
                 aria-hidden="true" />
              </button>
            )}

            {(collapsed || !isCollapsed) && (
              <div className="flex flex-col gap-0.5 mt-0.5">
                {section.links.filter(offered).map(({ href, labelKey, icon: Icon }) => {
                  const label = t(labelKey);
                  const active = isLinkActive(pathname, href);
                  return (
                    <Link
                      key={href}
                      to={href}
                      onClick={onNavigate}
                      className={`flex items-center gap-2.5 px-2 py-1.5 text-sm rounded-[var(--radius-sm)] transition-colors-fast overflow-hidden ${collapsed ? "justify-center" : ""} ${active ? "" : "hover:bg-[var(--surface-raised)]"}`}
                      style={{
                        backgroundColor: active ? "var(--accent)" : undefined,
                        color: active ? "white" : "var(--text)",
                      }}
                      title={collapsed ? label : undefined}
                    >
                      <Icon size={ICON.md} className="shrink-0" />
                      {!collapsed && <span className="min-w-0 flex-1">{label}</span>}
                    </Link>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
      {/* Plugin-contributed admin entries (slot admin.nav). Each granted plugin
          gets a sidebar link to its own dedicated full-page route
          (/admin/extensions/<id>), where its sandboxed iframe renders. */}
      {pluginEntries.length > 0 && (
        <div className="mt-2 flex flex-col gap-0.5">
          {pluginEntries.map((entry) => {
            const href = `/admin/extensions/${entry.pluginId}`;
            const active = isLinkActive(pathname, href);
            return (
              <Link
                key={entry.pluginId}
                to="/admin/extensions/$pluginId"
                params={{ pluginId: entry.pluginId }}
                onClick={onNavigate}
                className={`flex items-center gap-2.5 px-2 py-1.5 text-sm rounded-[var(--radius-sm)] transition-colors-fast overflow-hidden ${collapsed ? "justify-center" : ""} ${active ? "" : "hover:bg-[var(--surface-raised)]"}`}
                style={{
                  backgroundColor: active ? "var(--accent)" : undefined,
                  color: active ? "white" : "var(--text)",
                }}
                title={collapsed ? entry.name : undefined}
              >
                <Puzzle size={ICON.md} className="shrink-0" />
                {!collapsed && <span className="min-w-0 flex-1">{entry.name}</span>}
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
