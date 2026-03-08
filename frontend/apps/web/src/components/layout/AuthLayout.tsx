import { type ReactNode } from "react";
import { useRouterState } from "@tanstack/react-router";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";

interface AuthLayoutProps {
  children: ReactNode;
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { location } = useRouterState();
  const isHome = location.pathname === "/home";

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar />
      <div className="flex flex-col flex-1 overflow-hidden">
        <TopBar />
        <main className={`flex-1 min-h-0 overflow-y-auto${isHome ? "" : " p-6"}`}>
          {children}
        </main>
      </div>
    </div>
  );
}
