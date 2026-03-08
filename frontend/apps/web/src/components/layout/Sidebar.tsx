import { useRouterState } from "@tanstack/react-router";

const navItems = [
  { href: "/home", label: "Home" },
  { href: "/library", label: "Libraries" },
  { href: "/search", label: "Search" },
  { href: "/admin", label: "Admin" },
];

export function Sidebar() {
  const { location } = useRouterState();
  const pathname = location.pathname;

  return (
    <nav className="flex flex-col w-56 h-full bg-[var(--surface)] border-r border-[var(--border)] py-4 shrink-0">
      <div className="px-4 mb-6">
        <span className="text-xl font-bold text-[var(--accent)]">Evey</span>
      </div>
      <ul className="flex-1 flex flex-col gap-1 px-2">
        {navItems.map(({ href, label }) => {
          const active = pathname === href || pathname.startsWith(href + "/");
          return (
            <li key={href}>
              <a
                href={href}
                className={`flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] text-sm transition-colors ${
                  active
                    ? "bg-[var(--accent)] text-white"
                    : "text-[var(--text-muted)] hover:bg-[var(--surface-raised)] hover:text-[var(--text)]"
                }`}
              >
                {label}
              </a>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
