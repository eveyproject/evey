import { type ReactNode, useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import { Home, Library, Heart, Settings, Search, Menu } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { KeyboardShortcutsHelp } from "@/components/ui/KeyboardShortcutsHelp";

interface AuthLayoutProps {
  children: ReactNode;
}

const bottomNavItems = [
  { to: "/home" as const, labelKey: "common.nav.home", icon: Home },
  { to: "/library" as const, labelKey: "common.nav.libraries", icon: Library },
  { to: "/search" as const, labelKey: "common.nav.search", icon: Search },
  { to: "/favorites" as const, labelKey: "common.nav.favorites", icon: Heart },
];

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation();
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
        <div className="fixed inset-0 z-sidebar md:hidden">
          <div
            className="absolute inset-0 bg-black/50"
            onClick={() => setSidebarOpen(false)}
            role="button"
            aria-label={t("common.aria.closeMenu")}
            tabIndex={0}
            onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") setSidebarOpen(false); }}
          />
          <div className="relative z-overlay h-full w-56">
            <Sidebar onNavigate={() => setSidebarOpen(false)} />
          </div>
        </div>
      )}

      <div className="flex flex-col flex-1 overflow-hidden">
        {/* Hamburger + TopBar */}
        <div className="flex items-center">
          <button
            className="md:hidden p-3 text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast"
            onClick={() => setSidebarOpen(true)}
            aria-label={t("common.aria.openMenu")}
          >
            <Menu size={24} />
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
        aria-label={t("common.aria.bottomNav")}
        className="md:hidden fixed bottom-0 left-0 right-0 z-overlay flex items-center justify-around bg-[var(--surface)] border-t border-[var(--border)] h-16"
      >
        {bottomNavItems.map(({ to, labelKey, icon: Icon }) => {
          const active = location.pathname === to || location.pathname.startsWith(to + "/")
            || (to === "/library" && location.pathname.startsWith("/media/"));
          return (
            <Link
              key={to}
              to={to}
              preload="intent"
              className={`flex flex-col items-center gap-0.5 px-3 py-1 min-w-0 text-[11px] font-medium transition-colors-fast ${
                active ? "text-[var(--accent)]" : "text-[var(--text-muted)]"
              }`}
            >
              <Icon size={22} />
              <span>{t(labelKey)}</span>
            </Link>
          );
        })}
        {isAdmin && (
          <Link
            to="/admin"
            preload="intent"
            className={`flex flex-col items-center gap-0.5 px-3 py-1 min-w-0 text-[11px] font-medium transition-colors-fast ${
              location.pathname === "/admin" || location.pathname.startsWith("/admin/")
                ? "text-[var(--accent)]"
                : "text-[var(--text-muted)]"
            }`}
          >
            <Settings size={22} />
            <span>{t("common.nav.admin")}</span>
          </Link>
        )}
      </nav>

      <KeyboardShortcutsHelp open={showHelp} onClose={() => setShowHelp(false)} />
    </div>
  );
}
