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
  BarChart3,
  TrendingUp,
  FileText,
  Database,
  Cpu,
  SlidersHorizontal,
  ScrollText,
  ChevronDown,
  Sliders,
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
    titleKey: "nav.dashboardSystem",
    links: [
      { href: "/admin", labelKey: "nav.dashboard", icon: LayoutDashboard },
      { href: "/admin/sessions", labelKey: "nav.nowPlaying", icon: PlayCircle },
      { href: "/admin/remote-access", labelKey: "nav.remoteAccess", icon: Globe },
      { href: "/admin/server-config", labelKey: "nav.serverConfig", icon: SlidersHorizontal },
      { href: "/admin/system-health", labelKey: "nav.systemHealth", icon: Cpu },
    ],
  },
  {
    titleKey: "nav.media",
    links: [
      { href: "/admin/libraries", labelKey: "nav.libraries", icon: Library },
      { href: "/admin/metadata", labelKey: "nav.metadata", icon: Tags },
      { href: "/admin/transcode-cache", labelKey: "nav.transcodeCache", icon: HardDrive },
      { href: "/admin/transcoding-profiles", labelKey: "nav.transcodingProfiles", icon: Sliders },
    ],
  },
  {
    titleKey: "nav.usersAccess",
    links: [
      { href: "/admin/users", labelKey: "nav.users", icon: Users },
    ],
  },
  {
    titleKey: "nav.integrations",
    links: [
      { href: "/admin/plugins", labelKey: "nav.plugins", icon: Puzzle },
      { href: "/admin/webhooks", labelKey: "nav.webhooks", icon: Webhook },
    ],
  },
  {
    titleKey: "nav.monitoring",
    links: [
      { href: "/admin/analytics", labelKey: "nav.analytics", icon: TrendingUp },
      { href: "/admin/activity", labelKey: "nav.activity", icon: BarChart3 },
      { href: "/admin/audit", labelKey: "nav.auditLog", icon: FileText },
      { href: "/admin/logs", labelKey: "nav.liveLogs", icon: ScrollText },
    ],
  },
  {
    titleKey: "nav.maintenance",
    links: [
      { href: "/admin/backup", labelKey: "nav.backupRestore", icon: Database },
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
  const { t } = useTranslation("admin");
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
                className="w-full flex items-center justify-between px-2 py-1.5 cursor-pointer rounded-[var(--radius-sm)] transition-colors"
                style={{ color: "var(--text-muted)" }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.backgroundColor = "var(--surface-raised)";
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.backgroundColor = "transparent";
                }}
              >
                <span
                  className="text-[11px] font-semibold uppercase tracking-wider select-none"
                >
                  {sectionTitle}
                </span>
                <ChevronDown
                  size={12}
                  className="transition-transform"
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
                      className={`flex items-center gap-2.5 px-2 py-1.5 text-[13px] rounded-[var(--radius-sm)] transition-colors ${collapsed ? "justify-center" : ""}`}
                      style={{
                        backgroundColor: active ? "var(--accent)" : "transparent",
                        color: active ? "white" : "var(--text)",
                      }}
                      title={collapsed ? label : undefined}
                      onMouseEnter={(e) => {
                        if (!active) {
                          e.currentTarget.style.backgroundColor = "var(--surface-raised)";
                        }
                      }}
                      onMouseLeave={(e) => {
                        if (!active) {
                          e.currentTarget.style.backgroundColor = "transparent";
                        }
                      }}
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
