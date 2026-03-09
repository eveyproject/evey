import { useRef, useState, useEffect, useCallback } from "react";
import { Link, useNavigate, useRouterState } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { useUserServiceGetCurrentUser } from "@evey/api";
import { NotificationBell } from "@/features/user/NotificationBell";

function UserMenu({ userInitial }: { userInitial: string }) {
  const navigate = useNavigate();
  const { logout } = useAuthStore();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [open]);

  const go = (to: string) => {
    setOpen(false);
    void navigate({ to });
  };

  const handleLogout = () => {
    setOpen(false);
    logout();
    void navigate({ to: "/login" });
  };

  const items: { label: string; icon: string; action: () => void }[] = [
    { label: "Profile", icon: "M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z", action: () => go("/profile") },
    { label: "Settings", icon: "M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z M15 12a3 3 0 11-6 0 3 3 0 016 0z", action: () => go("/profile/settings") },
    { label: "Watch History", icon: "M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z", action: () => go("/profile/history") },
  ];

  if (isAdmin) {
    items.push({ label: "Admin", icon: "M10.5 6h9.75M10.5 6a1.5 1.5 0 11-3 0m3 0a1.5 1.5 0 10-3 0M3.75 6H7.5m3 12h9.75m-9.75 0a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m-3.75 0H7.5m9-6h3.75m-3.75 0a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m-9.75 0h9.75", action: () => go("/admin") });
  }

  return (
    <div ref={menuRef} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-8 h-8 rounded-full bg-[var(--accent)] flex items-center justify-center text-white text-sm font-medium cursor-pointer hover:opacity-90 transition-opacity"
        aria-label="User menu"
        aria-expanded={open}
        aria-haspopup="true"
      >
        {userInitial}
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-2 w-48 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] shadow-lg py-1 z-50 animate-fade-in">
          {items.map((item) => (
            <button
              key={item.label}
              onClick={item.action}
              className="w-full flex items-center gap-3 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface)] transition-colors cursor-pointer"
            >
              <svg className="w-4 h-4 text-[var(--text-muted)] shrink-0" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d={item.icon} />
              </svg>
              {item.label}
            </button>
          ))}
          <div className="border-t border-[var(--border)] my-1" />
          <button
            onClick={handleLogout}
            className="w-full flex items-center gap-3 px-3 py-2 text-sm text-red-400 hover:bg-[var(--surface)] transition-colors cursor-pointer"
          >
            <svg className="w-4 h-4 shrink-0" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15m3-3l3-3m0 0l-3-3m3 3H9" />
            </svg>
            Sign Out
          </button>
        </div>
      )}
    </div>
  );
}

export function TopBar() {
  const navigate = useNavigate();
  const { location } = useRouterState();
  const inputRef = useRef<HTMLInputElement>(null);
  const [searchValue, setSearchValue] = useState("");

  const { data: currentUser } = useUserServiceGetCurrentUser();
  const userInitial = currentUser?.username?.charAt(0).toUpperCase() ?? "U";

  const isOnSearch = location.pathname === "/search";

  const navigateToSearch = useCallback((q: string) => {
    const trimmed = q.trim();
    void navigate({ to: "/search", search: trimmed ? { q: trimmed } : {} });
  }, [navigate]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      navigateToSearch(searchValue);
      inputRef.current?.blur();
    }
    if (e.key === "Escape") {
      inputRef.current?.blur();
    }
  };

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        if (!isOnSearch) {
          void navigate({ to: "/search" });
        }
        setTimeout(() => inputRef.current?.focus(), 50);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [isOnSearch, navigate]);

  return (
    <header className="h-14 flex items-center justify-between px-3 sm:px-6 bg-[var(--surface)] border-b border-[var(--border)] shrink-0">
      <div className="flex-1">
        <div className="relative w-full max-w-64">
          <svg
            className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-muted)] pointer-events-none"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
            viewBox="0 0 24 24"
          >
            <circle cx="11" cy="11" r="8" />
            <path strokeLinecap="round" d="m21 21-4.35-4.35" />
          </svg>
          <input
            ref={inputRef}
            type="search"
            placeholder="Search..."
            aria-label="Search"
            value={searchValue}
            onChange={(e) => setSearchValue(e.target.value)}
            onKeyDown={handleKeyDown}
            onClick={() => { if (!isOnSearch) navigateToSearch(searchValue); }}
            className="w-full pl-9 pr-14 py-1.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
          />
          <kbd className="absolute right-2.5 top-1/2 -translate-y-1/2 hidden sm:inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[10px] font-medium text-[var(--text-muted)] bg-[var(--surface)] border border-[var(--border)] pointer-events-none">
            <span className="text-xs">&#8984;</span>K
          </kbd>
        </div>
      </div>
      <div className="flex items-center gap-3">
        <NotificationBell />
        <UserMenu userInitial={userInitial} />
      </div>
    </header>
  );
}
