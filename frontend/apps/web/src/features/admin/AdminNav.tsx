import { useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  LayoutDashboard,
  PlayCircle,
  Globe,
  Library,
  Tags,
  HardDrive,
  Users,
  Puzzle,
  Webhook,
  Key,
  BarChart3,
  TrendingUp,
  FileText,
  Database,
  Cpu,
  SlidersHorizontal,
  ScrollText,
  ChevronDown,
  Sliders,
  Rows3,
  type LucideIcon,
} from "lucide-react";

interface NavLink {
  href: string;
  labelKey: string;
  icon: LucideIcon;
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
      { href: "/admin/feeds", labelKey: "common.nav.homeFeeds", icon: Rows3 },
      { href: "/admin/system-health", labelKey: "admin.nav.systemHealth", icon: Cpu },
    ],
  },
  {
    titleKey: "admin.nav.media",
    links: [
      { href: "/admin/libraries", labelKey: "admin.nav.libraries", icon: Library },
      { href: "/admin/metadata", labelKey: "admin.nav.metadata", icon: Tags },
      { href: "/admin/transcode-cache", labelKey: "admin.nav.transcodeCache", icon: HardDrive },
      { href: "/admin/transcoding-profiles", labelKey: "admin.nav.transcodingProfiles", icon: Sliders },
    ],
  },
  {
    titleKey: "admin.nav.usersAccess",
    links: [
      { href: "/admin/users", labelKey: "admin.nav.users", icon: Users },
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
      { href: "/admin/audit", labelKey: "admin.nav.auditLog", icon: FileText },
      { href: "/admin/logs", labelKey: "admin.nav.liveLogs", icon: ScrollText },
    ],
  },
  {
    titleKey: "admin.nav.maintenance",
    links: [
      { href: "/admin/backup", labelKey: "admin.nav.backupRestore", icon: Database },
    ],
  },
];

export { sectionsDef as sections };

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
                className="w-full flex items-center justify-between px-2 py-1.5 cursor-pointer rounded-[var(--radius-sm)] transition-colors-fast hover:bg-[var(--surface-raised)]"
                style={{ color: "var(--text-muted)" }}
              >
                <span
                  className="text-[11px] font-semibold uppercase tracking-wider select-none"
                >
                  {sectionTitle}
                </span>
                <ChevronDown
                  size={12}
                  className="transition-transform-fast"
                  style={{
                    transform: isCollapsed ? "rotate(-90deg)" : "rotate(0deg)",
                  }}
                />
              </button>
            )}

            {(collapsed || !isCollapsed) && (
              <div className="flex flex-col gap-0.5 mt-0.5">
                {section.links.map(({ href, labelKey, icon: Icon }) => {
                  const label = t(labelKey);
                  const active = isLinkActive(pathname, href);
                  return (
                    <Link
                      key={href}
                      to={href}
                      onClick={onNavigate}
                      className={`flex items-center gap-2.5 px-2 py-1.5 text-[13px] rounded-[var(--radius-sm)] transition-colors-fast ${collapsed ? "justify-center" : ""} ${active ? "" : "hover:bg-[var(--surface-raised)]"}`}
                      style={{
                        backgroundColor: active ? "var(--accent)" : undefined,
                        color: active ? "white" : "var(--text)",
                      }}
                      title={collapsed ? label : undefined}
                    >
                      <Icon size={15} />
                      {!collapsed && <span>{label}</span>}
                    </Link>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
