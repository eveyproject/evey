import { Radio, Shield, Monitor, Smartphone, Tv } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { usePresenceServiceListOnline } from "@evey/api";
import type { V1OnlineUser } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/online")({
  component: OnlinePage,
});

const REFRESH_INTERVAL = 5_000;

function deviceIcon(ua: string) {
  const s = ua.toLowerCase();
  if (/(tv|tizen|webos|crkey|appletv)/.test(s)) return Tv;
  if (/(mobile|android|iphone|ipad)/.test(s)) return Smartphone;
  return Monitor;
}

// A short, human label for a raw user-agent (browser + OS best-effort).
function deviceLabel(ua: string): string {
  if (!ua) return "-";
  const s = ua.toLowerCase();
  let browser = "";
  if (s.includes("edg/")) browser = "Edge";
  else if (s.includes("chrome/") && !s.includes("chromium")) browser = "Chrome";
  else if (s.includes("firefox/")) browser = "Firefox";
  else if (s.includes("safari/") && !s.includes("chrome")) browser = "Safari";
  else if (s.startsWith("curl")) browser = "curl";
  let os = "";
  if (s.includes("windows")) os = "Windows";
  else if (s.includes("android")) os = "Android";
  else if (s.includes("iphone") || s.includes("ipad")) os = "iOS";
  else if (s.includes("mac os")) os = "macOS";
  else if (s.includes("linux")) os = "Linux";
  const label = [browser, os].filter(Boolean).join(" · ");
  return label || ua.slice(0, 40);
}

function OnlinePage() {
  const { t } = useTranslation();
  const { data, isLoading, isError } = usePresenceServiceListOnline({
    query: { refetchInterval: REFRESH_INTERVAL, refetchIntervalInBackground: false },
  });

  const users = data?.users ?? [];
  const total = data?.total ?? users.length;

  function lastSeen(u: V1OnlineUser): string {
    const idle = u.idleSeconds ?? 0;
    if (idle < 5) return t("admin.online.justNow");
    if (idle < 60) return t("admin.online.secondsAgo", { count: idle });
    return t("admin.online.minutesAgo", { count: Math.floor(idle / 60) });
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t("admin.online.title")}
        subtitle={t("admin.online.subtitle")}
      />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner />
        </div>
      ) : isError ? (
        <EmptyState title={t("admin.online.empty")} />
      ) : users.length === 0 ? (
        <EmptyState icon={<Radio size={40} />} title={t("admin.online.empty")} />
      ) : (
        <>
          <div className="flex items-center gap-2 text-sm text-[var(--text-muted)]">
            <span className="inline-flex h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
            {t(total === 1 ? "admin.online.countOne" : "admin.online.countOther", { count: total })}
          </div>

          <div className="overflow-x-auto rounded-xl border border-[var(--border)]">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[var(--text-muted)] border-b border-[var(--border)]">
                  <th className="px-4 py-3 font-medium">{t("admin.online.colUser")}</th>
                  <th className="px-4 py-3 font-medium">{t("admin.online.colDevice")}</th>
                  <th className="px-4 py-3 font-medium">{t("admin.online.colIp")}</th>
                  <th className="px-4 py-3 font-medium">{t("admin.online.colLastSeen")}</th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => {
                  const Icon = deviceIcon(u.userAgent ?? "");
                  return (
                    <tr key={u.userId} className="border-b border-[var(--border)] last:border-0">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2 font-medium text-[var(--text)]">
                          {u.username || u.userId}
                          {u.isAdmin && (
                            <Badge variant="info" className="gap-1">
                              <Shield size={11} />
                              {t("admin.online.adminBadge")}
                            </Badge>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-[var(--text-muted)]">
                        <div className="flex items-center gap-2">
                          <Icon size={14} />
                          {deviceLabel(u.userAgent ?? "")}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-[var(--text-muted)] font-mono text-xs">{u.clientIp || "-"}</td>
                      <td className="px-4 py-3 text-[var(--text-muted)]">{lastSeen(u)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
