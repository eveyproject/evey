import { useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
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
  FileText,
  ListChecks,
  ScrollText,
  ChevronDown,
  type LucideIcon,
} from "lucide-react";

interface NavLink {
  href: string;
  label: string;
  icon: LucideIcon;
}

interface NavSection {
  title: string;
  links: NavLink[];
}

const sections: NavSection[] = [
  {
    title: "Dashboard & System",
    links: [
      { href: "/admin", label: "Dashboard", icon: LayoutDashboard },
      { href: "/admin/sessions", label: "Now Playing", icon: PlayCircle },
      { href: "/admin/remote-access", label: "Remote Access", icon: Globe },
    ],
  },
  {
    title: "Media",
    links: [
      { href: "/admin/libraries", label: "Libraries", icon: Library },
      { href: "/admin/metadata", label: "Metadata", icon: Tags },
      { href: "/admin/transcode-cache", label: "Transcode Cache", icon: HardDrive },
    ],
  },
  {
    title: "Users & Access",
    links: [
      { href: "/admin/users", label: "Users", icon: Users },
    ],
  },
  {
    title: "Integrations",
    links: [
      { href: "/admin/plugins", label: "Plugins", icon: Puzzle },
      { href: "/admin/webhooks", label: "Webhooks", icon: Webhook },
    ],
  },
  {
    title: "Monitoring",
    links: [
      { href: "/admin/activity", label: "Activity", icon: BarChart3 },
      { href: "/admin/audit", label: "Audit Log", icon: FileText },
      { href: "/admin/tasks", label: "Tasks", icon: ListChecks },
      { href: "/admin/logs", label: "Logs", icon: ScrollText },
    ],
  },
];

function isLinkActive(pathname: string, href: string): boolean {
  if (href === "/admin") return pathname === "/admin" || pathname === "/admin/";
  return pathname === href || pathname.startsWith(href + "/");
}

export function AdminNav() {
  const { location } = useRouterState();
  const pathname = location.pathname;

  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  function toggleSection(title: string) {
    setCollapsed((prev) => ({ ...prev, [title]: !prev[title] }));
  }

  return (
    <nav
      className="w-56 shrink-0 rounded-[var(--radius-lg)] p-3 flex flex-col gap-1"
      style={{
        backgroundColor: "var(--surface)",
        border: "1px solid var(--border)",
      }}
    >
      {sections.map((section, idx) => {
        const isCollapsed = collapsed[section.title] ?? false;

        return (
          <div key={section.title} className={idx > 0 ? "mt-2" : ""}>
            <button
              type="button"
              onClick={() => toggleSection(section.title)}
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
                {section.title}
              </span>
              <ChevronDown
                size={12}
                className="transition-transform"
                style={{
                  transform: isCollapsed ? "rotate(-90deg)" : "rotate(0deg)",
                }}
              />
            </button>

            {!isCollapsed && (
              <div className="flex flex-col gap-0.5 mt-0.5">
                {section.links.map(({ href, label, icon: Icon }) => {
                  const active = isLinkActive(pathname, href);
                  return (
                    <Link
                      key={href}
                      to={href}
                      className="flex items-center gap-2.5 px-2 py-1.5 text-[13px] rounded-[var(--radius-sm)] transition-colors"
                      style={{
                        backgroundColor: active ? "var(--accent)" : "transparent",
                        color: active ? "white" : "var(--text)",
                      }}
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
                      <span>{label}</span>
                    </Link>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
    </nav>
  );
}
