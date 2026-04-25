import { useRef, useState, useEffect, useCallback } from "react";
import { useDebounce } from "@/hooks/useDebounce";
import type React from "react";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { Search } from "lucide-react";
import { NotificationBell } from "@/features/user/NotificationBell";
import { SearchDropdown } from "@/features/search/SearchDropdown";
import { useRecentSearches } from "@/hooks/useRecentSearches";
import { ProfileSwitcher } from "@/components/layout/ProfileSwitcher";

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
        <ProfileSwitcher profiles={profiles} activeProfileId={activeProfileId} />
      </div>
    </header>
  );
}
