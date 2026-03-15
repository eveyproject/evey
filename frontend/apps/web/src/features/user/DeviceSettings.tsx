import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";

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
    case "android": return "📱";
    default: return "🌐";
  }
}

function truncateToken(token: string): string {
  if (token.length <= 12) return token;
  return `${token.slice(0, 6)}…${token.slice(-6)}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

const DEVICES_QUERY_KEY = ["push-devices"] as const;

export function DeviceSettings() {
  const qc = useQueryClient();
  const [revokeTarget, setRevokeTarget] = useState<Device | null>(null);

  const { data, isLoading, refetch, isFetching } = useQuery({
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
      toast.success("Device revoked");
    },
    onError: (err: Error) => toast.error(err.message || "Failed to revoke device"),
  });

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">Registered Devices</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            Devices registered for push notifications. Revoke access for any device you no longer use.
          </p>
        </div>
        <Button variant="secondary" className="py-1 px-3 text-xs" loading={isFetching} onClick={() => void refetch()}>
          Refresh
        </Button>
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 text-sm text-[var(--text-muted)]">
          <span className="inline-block w-4 h-4 border-2 border-[var(--text-muted)] border-t-transparent rounded-full animate-spin" />
          Loading…
        </div>
      ) : devices.length === 0 ? (
        <div className="text-sm text-[var(--text-muted)]">No registered devices.</div>
      ) : (
        <table className="w-full text-sm border-collapse">
          <thead>
            <tr className="text-left text-xs text-[var(--text-muted)] border-b border-[var(--border)]">
              <th className="pb-2 font-semibold">Platform</th>
              <th className="pb-2 font-semibold">Token</th>
              <th className="pb-2 font-semibold">Registered</th>
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
                    Revoke
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Modal open={!!revokeTarget} onClose={() => setRevokeTarget(null)} title="Revoke Device">
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text)]">
            Revoke the <span className="font-semibold">{revokeTarget ? platformLabel(revokeTarget.platform) : ""}</span> device? It will no longer receive push notifications.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setRevokeTarget(null)}>
              Cancel
            </Button>
            <Button
              variant="danger"
              loading={revokeMutation.isPending}
              onClick={() => { if (revokeTarget) revokeMutation.mutate(revokeTarget.token); }}
            >
              Revoke
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
