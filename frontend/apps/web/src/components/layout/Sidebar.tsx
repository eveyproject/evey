import { Link, useRouterState } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";

const navItems = [
  { to: "/home" as const, label: "Home" },
  { to: "/library" as const, label: "Libraries" },
  { to: "/favorites" as const, label: "Favorites" },
  { to: "/collections" as const, label: "Collections" },
  { to: "/search" as const, label: "Search" },
];

export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { location } = useRouterState();
  const pathname = location.pathname;
  const isAdmin = useAuthStore((s) => s.isAdmin);

  return (
    <nav aria-label="Main navigation" className="flex flex-col w-56 h-full bg-[var(--surface)] border-r border-[var(--border)] py-4 shrink-0">
      <div className="px-4 mb-6">
        <span className="text-xl font-bold text-[var(--accent)]">Evey</span>
      </div>
      <ul className="flex-1 flex flex-col gap-1 px-2">
        {navItems.map(({ to, label }) => {
          const active = pathname === to || pathname.startsWith(to + "/")
            || (to === "/library" && pathname.startsWith("/media/"));
          return (
            <li key={to}>
              <Link
                to={to}
                onClick={onNavigate}
                className={`flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors ${
                  active
                    ? "border-l-2 border-[var(--accent)] text-[var(--accent)] bg-[var(--surface-raised)]"
                    : "border-l-2 border-transparent text-[var(--text-secondary)] hover:bg-[var(--surface-raised)] hover:text-[var(--text)]"
                }`}
              >
                {label}
              </Link>
            </li>
          );
        })}
        {isAdmin && (
          <li>
            <Link
              to="/admin"
              onClick={onNavigate}
              className={`flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors ${
                pathname === "/admin" || pathname.startsWith("/admin/")
                  ? "border-l-2 border-[var(--accent)] text-[var(--accent)] bg-[var(--surface-raised)]"
                  : "border-l-2 border-transparent text-[var(--text-secondary)] hover:bg-[var(--surface-raised)] hover:text-[var(--text)]"
              }`}
            >
              Admin
            </Link>
          </li>
        )}
      </ul>
    </nav>
  );
}
