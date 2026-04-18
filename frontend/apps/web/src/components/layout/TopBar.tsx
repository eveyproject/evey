import { useRef, useState, useEffect, useCallback, useMemo } from "react";
import { useDebounce } from "@/hooks/useDebounce";
import type React from "react";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, userServiceLogout } from "@evey/api";
import { Search, Settings, Clock, LayoutList, LogOut, QrCode, BarChart3 } from "lucide-react";
import { NotificationBell } from "@/features/user/NotificationBell";
import { SearchDropdown } from "@/features/search/SearchDropdown";
import { useRecentSearches } from "@/hooks/useRecentSearches";
import { ProfileSwitcher } from "@/components/layout/ProfileSwitcher";

function UserMenu({ userInitial, avatarUrl }: { userInitial: string; avatarUrl: string | null }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { logout } = useAuthStore();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onMouseDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const go = useCallback((to: string) => {
    setOpen(false);
    void navigate({ to });
  }, [navigate]);

  const handleLogout = () => {
    setOpen(false);
    queryClient.clear();
    logout();
    localStorage.removeItem("evey-recent-searches");
    // Clear httpOnly refresh cookie server-side (best-effort)
    void userServiceLogout({}).catch((err) => console.warn("logout failed", err));
    void navigate({ to: "/login" });
  };

  const items = useMemo(() => {
    const base: { label: string; icon: React.ReactNode; action: () => void }[] = [
      { label: t("common.nav.settings"), icon: <Settings size={16} />, action: () => go("/profile/settings") },
      { label: t("common.nav.watchHistory"), icon: <Clock size={16} />, action: () => go("/profile/history") },
      { label: t("common.nav.myStats"), icon: <BarChart3 size={16} />, action: () => go("/profile/stats") },
      { label: t("common.nav.quickConnect"), icon: <QrCode size={16} />, action: () => go("/quickconnect") },
    ];
    if (isAdmin) {
      base.push({ label: t("common.nav.admin"), icon: <LayoutList size={16} />, action: () => go("/admin") });
    }
    return base;
  }, [go, isAdmin, t]);

  return (
    <div ref={menuRef} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-8 h-8 rounded-full bg-[var(--accent)] flex items-center justify-center text-[var(--filter-active-text)] text-sm font-medium cursor-pointer hover:opacity-90 transition-opacity-fast overflow-hidden"
        aria-label={t("common.nav.userMenu")}
        aria-expanded={open}
        aria-haspopup="true"
      >
        {avatarUrl ? (
          <img src={avatarUrl} alt="" className="w-full h-full object-cover" />
        ) : (
          userInitial
        )}
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-2 w-48 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] shadow-lg py-1 z-overlay animate-fade-in">
          {items.map((item) => (
            <button
              key={item.label}
              onClick={item.action}
              className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer"
            >
              <span className="text-[var(--text-muted)] shrink-0">{item.icon}</span>
              {item.label}
            </button>
          ))}
          <div className="border-t border-[var(--border)] my-1" />
          <button
            onClick={handleLogout}
            data-testid="logout-btn"
            className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--color-danger)] hover:bg-[var(--surface)] transition-colors-fast cursor-pointer"
          >
            <LogOut size={16} className="shrink-0" />
            {t("common.nav.signOut")}
          </button>
        </div>
      )}
    </div>
  );
}

export function TopBar() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { location } = useRouterState();
  const inputRef = useRef<HTMLInputElement>(null);
  const [searchValue, setSearchValue] = useState("");
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const debouncedSearch = useDebounce(searchValue.trim(), 200);
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const main = document.querySelector("main");
    if (!main) return;
    let raf = 0;
    const handler = () => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        setScrolled(main.scrollTop > 8);
        raf = 0;
      });
    };
    main.addEventListener("scroll", handler, { passive: true });
    return () => {
      main.removeEventListener("scroll", handler);
      cancelAnimationFrame(raf);
    };
  }, []);

  const { data: currentUser } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(currentUser?.id ?? "", { query: { enabled: !!currentUser?.id } });
  const profiles = profilesResp?.profiles ?? [];
  const activeProfileId = useAuthStore((s) => s.profileId);
  const activeProfile = profiles.find((p) => p.id === activeProfileId)
    ?? profiles.find((p) => p.isDefault)
    ?? profiles[0];
  const userInitial = currentUser?.username?.charAt(0).toUpperCase() ?? "U";
  const userAvatarUrl = activeProfile?.avatarPath && activeProfile?.id
    ? `/api/v1/profiles/${activeProfile.id}/avatar`
    : null;
  const hasMultipleProfiles = profiles.length >= 2;
  const { recents, push: pushRecent, remove: removeRecent, clear: clearRecents } = useRecentSearches();

  const isOnSearch = location.pathname === "/search";

  const navigateToSearch = useCallback((q: string) => {
    const trimmed = q.trim();
    if (trimmed) pushRecent(trimmed);
    setDropdownOpen(false);
    void navigate({ to: "/search", search: trimmed ? { q: trimmed } : {} });
  }, [navigate, pushRecent]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      navigateToSearch(searchValue);
      inputRef.current?.blur();
    }
    if (e.key === "Escape") {
      setDropdownOpen(false);
      inputRef.current?.blur();
    }
    if (e.key === "ArrowDown" && dropdownOpen) {
      e.preventDefault();
    }
  };

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        inputRef.current?.focus();
        setDropdownOpen(true);
        return;
      }
      if (e.key === "/" && !e.metaKey && !e.ctrlKey && !e.altKey) {
        const tag = (e.target as HTMLElement)?.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || (e.target as HTMLElement)?.isContentEditable) return;
        e.preventDefault();
        inputRef.current?.focus();
        setDropdownOpen(true);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  // Close dropdown when navigating away
  useEffect(() => {
    setDropdownOpen(false);
  }, [location.pathname]);

  return (
    <header
      className="h-14 flex items-center justify-between px-3 sm:px-6 border-b shrink-0 transition-all duration-200"
      style={{
        backgroundColor: scrolled ? "var(--glass-bg)" : "var(--surface)",
        backdropFilter: scrolled ? `blur(var(--glass-blur))` : "none",
        borderColor: scrolled ? "var(--glass-border)" : "var(--border)",
      }}
    >
      <div className="flex-1">
        <div className="relative w-full max-w-[60vw] sm:max-w-80">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-muted)] pointer-events-none z-raised" size={16} />
          <input
            ref={inputRef}
            type="search"
            role="combobox"
            aria-expanded={dropdownOpen && !isOnSearch}
            aria-controls="search-results"
            aria-autocomplete="list"
            placeholder={t("media.search.placeholder")}
            aria-label={t("common.search")}
            data-testid="topbar-search-input"
            value={searchValue}
            onChange={(e) => {
              setSearchValue(e.target.value);
              if (e.target.value.trim().length > 1) setDropdownOpen(true);
            }}
            onFocus={() => {
              if (searchValue.trim().length > 1 || recents.length > 0) setDropdownOpen(true);
            }}
            onKeyDown={handleKeyDown}
            className="w-full pl-9 pr-14 py-1.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
          />
          <kbd className="absolute right-2.5 top-1/2 -translate-y-1/2 hidden sm:inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-xs font-medium text-[var(--text-muted)] bg-[var(--surface)] border border-[var(--border)] pointer-events-none">
            {/Mac|iPhone|iPad/.test(navigator.userAgent) ? <><span className="text-xs">&#8984;</span>K</> : <><span className="text-xs">Ctrl+</span>K</>}
          </kbd>

          <SearchDropdown
            query={debouncedSearch}
            visible={dropdownOpen && !isOnSearch}
            onClose={() => setDropdownOpen(false)}
            onNavigateToSearch={navigateToSearch}
            inputRef={inputRef}
            recents={recents}
            onRemoveRecent={removeRecent}
            onClearRecents={clearRecents}
          />
        </div>
      </div>
      <div className="flex items-center gap-3">
        <NotificationBell />
        {hasMultipleProfiles && (
          <ProfileSwitcher profiles={profiles} activeProfileId={activeProfileId} />
        )}
        <UserMenu userInitial={userInitial} avatarUrl={userAvatarUrl} />
      </div>
    </header>
  );
}
