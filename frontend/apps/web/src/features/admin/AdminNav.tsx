import { Link, useRouterState } from "@tanstack/react-router";

const adminNavLinks = [
  { href: "/admin/libraries", label: "Libraries" },
  { href: "/admin/users", label: "Users" },
  { href: "/admin/plugins", label: "Plugins" },
  { href: "/admin/webhooks", label: "Webhooks" },
  { href: "/admin/audit", label: "Audit Log" },
  { href: "/admin/metadata", label: "Metadata" },
  { href: "/admin/activity", label: "Activity" },
  { href: "/admin/tasks", label: "Scheduled Tasks" },
  { href: "/admin/transcode-cache", label: "Transcode Cache" },
  { href: "/admin/logs", label: "Logs" },
];

export function AdminNav() {
  const { location } = useRouterState();
  const pathname = location.pathname;

  return (
    <div className="flex flex-wrap gap-2">
      {adminNavLinks.map(({ href, label }) => {
        const isActive = pathname === href || (href !== "/admin" && pathname.startsWith(href));
        return (
          <Link
            key={href}
            to={href}
            className="px-4 py-2 text-sm font-medium rounded-[var(--radius-md)] transition-colors"
            style={{
              backgroundColor: isActive ? "var(--accent)" : "var(--surface)",
              border: `1px solid ${isActive ? "var(--accent)" : "var(--border)"}`,
              color: isActive ? "white" : "var(--text)",
            }}
          >
            {label}
          </Link>
        );
      })}
    </div>
  );
}
