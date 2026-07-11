import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { eveyFetch } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";

type Device = {
  token: string;
  platform: string;
  createdAt: string;
};

function platformLabel(platform: string): string {
  switch (platform.toLowerCase()) {
    case "ios": return "iOS";
    case "android": return "Android";
    case "web": return "Web";
    default: return platform;
  }
}

function platformIcon(platform: string): string {
  switch (platform.toLowerCase()) {
    case "ios":
    case "android": return "\uD83D\uDCF1";
    default: return "\uD83C\uDF10";
  }
}

function truncateToken(token: string): string {
  if (token.length <= 12) return token;
  return `${token.slice(0, 6)}\u2026${token.slice(-6)}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

const DEVICES_QUERY_KEY = ["push-devices"] as const;

// The list is read-only: native device-token registration (FCM-era) was
// removed server-side, so this can only ever render the empty state until a
// native push backend exists. The revoke flow that used to live here called
// an endpoint that no longer exists.
export function DeviceSettings() {
  const { t } = useTranslation();

  const { data, isLoading, isError, refetch, isFetching } = useQuery({
    queryKey: DEVICES_QUERY_KEY,
    queryFn: () =>
      eveyFetch<{ devices: Device[] }>({
        url: "/api/v1/push/devices",
        method: "GET",
      }),
  });

  const devices = data?.devices ?? [];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.devices.title")}</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {t("auth.devices.description")}
          </p>
        </div>
        <Button variant="secondary" className="py-1 px-3 text-xs" loading={isFetching} onClick={() => void refetch()}>
          {t("common.refresh")}
        </Button>
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 text-sm text-[var(--text-muted)]">
          <span className="inline-block w-4 h-4 border-2 border-[var(--text-muted)] border-t-transparent rounded-full animate-spin" />
          {t("common.loading")}
        </div>
      ) : isError ? (
        <EmptyState
          icon={<TriangleAlert size={32} />}
          title={t("common.errorBoundary.title")}
          description={t("common.errorBoundary.description")}
        />
      ) : devices.length === 0 ? (
        <div className="text-sm text-[var(--text-muted)]">{t("auth.devices.noDevices")}</div>
      ) : (
        <table className="w-full text-sm border-collapse">
          <thead>
            <tr className="text-left text-xs text-[var(--text-muted)] border-b border-[var(--border)]">
              <th className="pb-2 font-semibold">{t("auth.devices.platform")}</th>
              <th className="pb-2 font-semibold">{t("auth.devices.token")}</th>
              <th className="pb-2 font-semibold">{t("auth.devices.registered")}</th>
            </tr>
          </thead>
          <tbody>
            {devices.map((d) => (
              <tr key={d.token} className="border-b border-[var(--border)] last:border-0">
                <td className="py-3 pr-4 font-medium text-[var(--text)]">
                  <span className="mr-1">{platformIcon(d.platform)}</span>{platformLabel(d.platform)}
                </td>
                <td className="py-3 pr-4 font-mono text-xs text-[var(--text-muted)]">{truncateToken(d.token)}</td>
                <td className="py-3 pr-4 text-[var(--text-muted)]">{formatDate(d.createdAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
