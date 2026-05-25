import { type ReactNode, useState } from "react";
import { useRouterState } from "@tanstack/react-router";
import { Menu } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { KeyboardShortcutsHelp } from "@/components/ui/KeyboardShortcutsHelp";
import { GlobalAudioPlayer } from "@/features/music/GlobalAudioPlayer";
import { useMusicPlayerStore } from "@evey/store";

interface AuthLayoutProps {
  children: ReactNode;
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation();
  const { location } = useRouterState();
  const isHome = location.pathname === "/home";
  const isPlayer = location.pathname.startsWith("/player/");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const { showHelp, setShowHelp } = useKeyboardShortcuts(!isPlayer);
  // The audio player is a fixed bottom bar; when a track is loaded it would
  // overlap the bottom of the scrollable content. Reserve its height so the
  // last rows / action buttons stay reachable.
  const musicActive = useMusicPlayerStore((s) => s.playing) !== null;

  if (isPlayer) return <>{children}</>;

  return (
    <div className="flex h-screen overflow-hidden">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:z-popover focus:top-2 focus:left-2 focus:px-4 focus:py-2 focus:rounded-[var(--radius-md)] focus:bg-[var(--accent)] focus:text-white focus:text-sm focus:font-medium"
      >
        {t("common.aria.skipToContent")}
      </a>

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
            data-testid="close-sidebar-overlay"
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
            data-testid="open-sidebar-btn"
          >
            <Menu size={24} />
          </button>
          <div className="flex-1">
            <TopBar />
          </div>
        </div>
        <main
          id="main-content"
          key={location.pathname}
          className={`flex-1 min-h-0 overflow-y-auto animate-fade-in${isHome ? "" : " p-4 sm:p-6"}${musicActive ? " pb-24" : ""}`}
        >
          {children}
        </main>
      </div>

      <KeyboardShortcutsHelp open={showHelp} onClose={() => setShowHelp(false)} />
      <GlobalAudioPlayer />
    </div>
  );
}
