import { useState, useCallback, useEffect } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { EveyWordmark, EveyMark } from "@/components/ui/EveyLogo";
import {
  Home,
  Library,
  Heart,
  Music2,
  Images,
  Settings,
  ArrowLeft,
  Headphones,
} from "lucide-react";
import { AdminNav } from "@/features/admin/AdminNav";
import { PluginSlot } from "@/features/plugins/PluginSlot";

const COLLAPSED_KEY = "evey-sidebar-collapsed";

const navItems = [
  { to: "/home" as const, labelKey: "common.nav.home", icon: Home },
  { to: "/library" as const, labelKey: "common.nav.libraries", icon: Library },
  { to: "/photos" as const, labelKey: "common.nav.photos", icon: Images },
  { to: "/favorites" as const, labelKey: "common.nav.favorites", icon: Heart },
  { to: "/music" as const, labelKey: "common.nav.music", icon: Music2 },
  { to: "/syncplay" as const, labelKey: "common.nav.syncplay", icon: Headphones },
];

export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation();
  const { location } = useRouterState();
  const pathname = location.pathname;
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const isOnAdmin = pathname === "/admin" || pathname.startsWith("/admin/");

  const [collapsed, setCollapsed] = useState(true);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((prev) => {
      const next = !prev;
      try { localStorage.setItem(COLLAPSED_KEY, String(next)); } catch (err) { console.warn("localStorage write failed", err); }
      return next;
    });
  }, []);

  useEffect(() => {
    if (isOnAdmin) setCollapsed(false);
  }, [isOnAdmin]);

  const isMobile = !!onNavigate;
  const isCollapsed = isMobile ? false : collapsed;

  return (
    <nav
      aria-label={isOnAdmin ? t("common.nav.adminNav") : t("common.nav.mainNav")}
      className={`flex flex-col h-full bg-[var(--surface)] border-r border-[var(--border)] py-4 shrink-0 overflow-hidden transition-[max-width] duration-200 ${isCollapsed ? "max-w-16 w-16" : "max-w-56 w-56"}`}
    >
      {/* Header — on desktop the logo itself toggles the collapsed state
          (the old dedicated button is gone); on the mobile overlay there's no
          collapse, so the logo links home and closes the drawer. */}
      <div className={`flex items-center mb-6 ${isCollapsed ? "justify-center px-2" : "justify-between px-4"}`}>
        {isMobile ? (
          <Link
            to="/home"
            onClick={onNavigate}
            preload="intent"
            aria-label="Evey"
            className="text-[var(--text)] hover:opacity-80 transition-opacity-fast"
          >
            <EveyWordmark height={24} />
          </Link>
        ) : (
          <button
            onClick={toggleCollapsed}
            aria-label={isCollapsed ? t("common.nav.expandSidebar") : t("common.nav.collapseSidebar")}
            title={isCollapsed ? t("common.nav.expandSidebar") : t("common.nav.collapseSidebar")}
            className="text-[var(--text)] hover:opacity-80 transition-opacity-fast cursor-pointer"
          >
            {isCollapsed ? <EveyMark height={28} /> : <EveyWordmark height={24} />}
          </button>
        )}
      </div>

      {isOnAdmin && isAdmin ? (
        // Admin nav mode
        <div className="flex-1 flex flex-col overflow-y-auto">
          {/* Back to app link */}
          <div className={`mb-3 ${isCollapsed ? "px-2" : "px-2"}`}>
            <Link
              to="/home"
              onClick={onNavigate}
              preload="intent"
              className={`flex items-center gap-2.5 px-2 py-1.5 text-sm rounded-[var(--radius-sm)] transition-colors-fast text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-raised)] ${isCollapsed ? "justify-center" : ""}`}
              title={isCollapsed ? t("common.nav.backToApp") : undefined}
            >
              <ArrowLeft size={15} />
              {!isCollapsed && <span>{t("common.nav.backToApp")}</span>}
            </Link>
            <div className="border-b border-[var(--border)] mt-2" />
          </div>

          <div className={`flex-1 ${isCollapsed ? "px-1" : "px-2"}`}>
            <AdminNav collapsed={isCollapsed} onNavigate={onNavigate} />
          </div>
        </div>
      ) : (
        // Regular nav mode
        <ul className={`flex-1 flex flex-col gap-1 ${isCollapsed ? "px-1" : "px-2"}`}>
          {navItems.map(({ to, labelKey, icon: Icon }) => {
            const label = t(labelKey);
            const active = pathname === to || pathname.startsWith(to + "/")
              || (to === "/library" && pathname.startsWith("/media/"));
            return (
              <li key={to}>
                <Link
                  to={to}
                  onClick={onNavigate}
                  preload="intent"
                  className={`flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast overflow-hidden ${isCollapsed ? "justify-center" : ""} ${
                    active
                      ? "border-l-2 border-[var(--accent)] text-[var(--accent)] bg-[var(--surface-raised)]"
                      : "border-l-2 border-transparent text-[var(--text-secondary)] hover:bg-[var(--surface-raised)] hover:text-[var(--text)]"
                  }`}
                  title={isCollapsed ? label : undefined}
                >
                  <Icon size={18} />
                  {!isCollapsed && <span className="whitespace-nowrap">{label}</span>}
                </Link>
              </li>
            );
          })}
          {isAdmin && (
            <li>
              <Link
                to="/admin"
                onClick={onNavigate}
                preload="intent"
                className={`flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast overflow-hidden ${isCollapsed ? "justify-center" : ""} ${
                  isOnAdmin
                    ? "border-l-2 border-[var(--accent)] text-[var(--accent)] bg-[var(--surface-raised)]"
                    : "border-l-2 border-transparent text-[var(--text-secondary)] hover:bg-[var(--surface-raised)] hover:text-[var(--text)]"
                }`}
                title={isCollapsed ? t("common.nav.admin") : undefined}
              >
                <Settings size={18} />
                {!isCollapsed && <span className="whitespace-nowrap">{t("common.nav.admin")}</span>}
              </Link>
            </li>
          )}
        </ul>
      )}
      <div className={`mt-auto ${isCollapsed ? "px-1" : "px-2"}`}>
        <PluginSlot name="sidebar.bottom" />
      </div>
    </nav>
  );
}
