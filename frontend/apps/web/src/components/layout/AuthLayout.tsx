import { type ReactNode, useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import { Home, Library, Heart, Settings, Search } from "lucide-react";
import { useAuthStore } from "@evey/store";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { KeyboardShortcutsHelp } from "@/components/ui/KeyboardShortcutsHelp";

interface AuthLayoutProps {
  children: ReactNode;
}

const bottomNavItems = [
  { to: "/home" as const, label: "Home", icon: Home },
  { to: "/library" as const, label: "Libraries", icon: Library },
  { to: "/search" as const, label: "Search", icon: Search },
  { to: "/favorites" as const, label: "Favorites", icon: Heart },
];

export function AuthLayout({ children }: AuthLayoutProps) {
  const { location } = useRouterState();
  const isHome = location.pathname === "/home";
  const isPlayer = location.pathname.startsWith("/player/");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const { showHelp, setShowHelp } = useKeyboardShortcuts(!isPlayer);

  if (isPlayer) return <>{children}</>;

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Desktop sidebar */}
      <div className="hidden md:block">
        <Sidebar />
      </div>

      {/* Mobile sidebar overlay */}
      {sidebarOpen && (
        <div className="fixed inset-0 z-40 md:hidden">
          <div
            className="absolute inset-0 bg-black/50"
            onClick={() => setSidebarOpen(false)}
            role="button"
            aria-label="Close menu"
            tabIndex={0}
            onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") setSidebarOpen(false); }}
          />
          <div className="relative z-50 h-full w-56">
            <Sidebar onNavigate={() => setSidebarOpen(false)} />
          </div>
        </div>
      )}

      <div className="flex flex-col flex-1 overflow-hidden">
        {/* Hamburger + TopBar */}
        <div className="flex items-center">
          <button
            className="md:hidden p-3 text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
            onClick={() => setSidebarOpen(true)}
            aria-label="Open menu"
          >
            <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
            </svg>
          </button>
          <div className="flex-1">
            <TopBar />
          </div>
        </div>
        <main
          key={location.pathname}
          className={`flex-1 min-h-0 overflow-y-auto animate-fade-in${isHome ? " pb-16 md:pb-0" : " p-4 sm:p-6 pb-20 md:pb-6"}`}
        >
          {children}
        </main>
      </div>

      {/* Bottom navigation — mobile only */}
      <nav
        aria-label="Bottom navigation"
        className="md:hidden fixed bottom-0 left-0 right-0 z-50 flex items-center justify-around bg-[var(--surface)] border-t border-[var(--border)] h-16"
      >
        {bottomNavItems.map(({ to, label, icon: Icon }) => {
          const active = location.pathname === to || location.pathname.startsWith(to + "/")
            || (to === "/library" && location.pathname.startsWith("/media/"));
          return (
            <Link
              key={to}
              to={to}
              className={`flex flex-col items-center gap-0.5 px-3 py-1 min-w-0 text-[11px] font-medium transition-colors ${
                active ? "text-[var(--accent)]" : "text-[var(--text-muted)]"
              }`}
            >
              <Icon size={22} />
              <span>{label}</span>
            </Link>
          );
        })}
        {isAdmin && (
          <Link
            to="/admin"
            className={`flex flex-col items-center gap-0.5 px-3 py-1 min-w-0 text-[11px] font-medium transition-colors ${
              location.pathname === "/admin" || location.pathname.startsWith("/admin/")
                ? "text-[var(--accent)]"
                : "text-[var(--text-muted)]"
            }`}
          >
            <Settings size={22} />
            <span>Admin</span>
          </Link>
        )}
      </nav>

      <KeyboardShortcutsHelp open={showHelp} onClose={() => setShowHelp(false)} />
    </div>
  );
}
