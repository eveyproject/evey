import { type ReactNode, useEffect, useRef, useState } from "react";
import { useFocusTrap } from "@/hooks/useFocusTrap";
import { useRouterState } from "@tanstack/react-router";
import { Menu } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";
import { OfflineBanner } from "./OfflineBanner";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { KeyboardShortcutsHelp } from "@/components/ui/KeyboardShortcutsHelp";
import { GlobalAudioPlayer } from "@/features/music/GlobalAudioPlayer";
import { useMusicPlayerStore } from "@evey/store";
import { ProfileLockOverlay } from "@/features/profile/ProfileLockOverlay";

interface AuthLayoutProps {
  children: ReactNode;
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation();
  const { location } = useRouterState();
  const isHome = location.pathname === "/home";
  const isPlayer = location.pathname.startsWith("/player/");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  // The phone-width drawer is a dialog in all but name: Escape closes it and
  // Tab stays inside it while it is open, as the modals already do.
  const drawerRef = useRef<HTMLDivElement>(null);
  useFocusTrap(drawerRef, sidebarOpen);
  useEffect(() => {
    if (!sidebarOpen) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setSidebarOpen(false); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [sidebarOpen]);
  const { showHelp, setShowHelp } = useKeyboardShortcuts(!isPlayer);
  // The audio player is a fixed bottom bar; when a track is loaded it would
  // overlap the bottom of the scrollable content. Reserve its height so the
  // last rows / action buttons stay reachable.
  const musicActive = useMusicPlayerStore((s) => s.playing) !== null;

  // Navigation used to remount <main> wholesale (key={pathname}) just to
  // reset the scroll and replay the fade. On a heavy page (home = 100+
  // cards) that unmount/remount burned the main thread right when the user
  // tapped - the "hard to click" jank. Reproduce both effects without the
  // remount: scroll to top and replay a cheap opacity animation.
  const mainRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const el = mainRef.current;
    if (!el) return;
    // Feature-detected: a test DOM may implement neither Element.scrollTo
    // nor the Web Animations API.
    if (typeof el.scrollTo === "function") el.scrollTo({ top: 0 });
    if (typeof el.animate !== "function") return;
    const anim = el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 200, easing: "ease-out" });
    // Cancelling rejects `finished`; nobody awaits it, so mark it handled
    // (browsers do so themselves, happy-dom reports it as unhandled).
    anim.finished?.catch(() => {});
    return () => anim.cancel();
  }, [location.pathname]);

  // The player skips the chrome but not the lock: a closed profile is
  // stopped mid-film too.
  if (isPlayer) return <>{children}<ProfileLockOverlay /></>;

  return (
    <div className="flex h-dvh overflow-hidden">
      <OfflineBanner />
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:z-popover focus:top-2 focus:left-2 focus:px-4 focus:py-2 focus:rounded-[var(--radius-md)] focus:bg-[var(--accent)] focus:text-[var(--filter-active-text)] focus:text-sm focus:font-medium"
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
          <div ref={drawerRef} className="relative z-overlay h-full w-56">
            <Sidebar onNavigate={() => setSidebarOpen(false)} />
          </div>
        </div>
      )}

      <div className="flex flex-col flex-1 overflow-hidden">
        {/* Hamburger + TopBar - chrome level (z-sidebar) so the TopBar's
            absolutely-positioned dropdowns (notification bell, search results)
            paint above <main> content but below modals/popovers. Without a
            stacking position here, <main> (the next DOM sibling, an overflow
            scroll container) renders over the dropdowns and hides them. */}
        <div className="relative z-sidebar flex items-center">
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
          ref={mainRef}
          className={`flex-1 min-h-0 overflow-y-auto${isHome ? "" : " p-4 sm:p-6"}${musicActive ? " pb-24" : ""}`}
        >
          {children}
        </main>
      </div>

      <KeyboardShortcutsHelp open={showHelp} onClose={() => setShowHelp(false)} />
      <GlobalAudioPlayer />
      <ProfileLockOverlay />
    </div>
  );
}
