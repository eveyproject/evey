import { type ReactNode, useState } from "react";
import { useRouterState } from "@tanstack/react-router";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";

interface AuthLayoutProps {
  children: ReactNode;
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { location } = useRouterState();
  const isHome = location.pathname === "/home";
  const isPlayer = location.pathname.startsWith("/player/");
  const [sidebarOpen, setSidebarOpen] = useState(false);

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
        <main className={`flex-1 min-h-0 overflow-y-auto${isHome ? "" : " p-6"}`}>
          {children}
        </main>
      </div>
    </div>
  );
}
