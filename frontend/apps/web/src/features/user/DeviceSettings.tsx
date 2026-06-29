import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
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

export function DeviceSettings() {
  const { t } = useTranslation();
    const qc = useQueryClient();
  const [revokeTarget, setRevokeTarget] = useState<Device | null>(null);

  const { data, isLoading, isError, refetch, isFetching } = useQuery({
    queryKey: DEVICES_QUERY_KEY,
    queryFn: () =>
      eveyFetch<{ devices: Device[] }>({
        url: "/api/v1/push/devices",
        method: "GET",
      }),
  });

  const devices = data?.devices ?? [];

  const revokeMutation = useMutation({
    mutationFn: (token: string) =>
      eveyFetch({
        url: `/api/v1/push/devices/${encodeURIComponent(token)}`,
        method: "DELETE",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: DEVICES_QUERY_KEY });
      setRevokeTarget(null);
      toast.success(t("auth.devices.deviceRevoked"));
    },
    onError: (err: Error) => toast.error(err.message || t("auth.devices.revokeError")),
  });

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
              <th className="pb-2" />
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
                <td className="py-3">
                  <Button
                    variant="danger"
                    className="py-1 px-2 text-xs"
                    onClick={() => setRevokeTarget(d)}
                  >
                    {t("common.revoke")}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Modal open={!!revokeTarget} onClose={() => setRevokeTarget(null)} title={t("auth.devices.revokeTitle")}>
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text)]">
            {t("auth.devices.revokeConfirm", { platform: revokeTarget ? platformLabel(revokeTarget.platform) : "" })}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setRevokeTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="danger"
              loading={revokeMutation.isPending}
              onClick={() => { if (revokeTarget) revokeMutation.mutate(revokeTarget.token); }}
            >
              {t("common.revoke")}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
