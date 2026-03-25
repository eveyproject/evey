import { useState, useEffect } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";

export const Route = createLazyFileRoute("/_auth/admin/remote-access")({
  component: RemoteAccessPage,
});

interface RemoteAccessConfig {
  enabled: boolean;
  externalUrl: string;
  lanNetworks: string[];
  maxRemoteBitrateKbps: number;
}

interface NetworkInterface {
  name: string;
  address: string;
}

const CONFIG_KEY = ["/api/v1/admin/remote-access"];
const NETWORK_KEY = ["/api/v1/admin/network-info"];

function useRemoteAccessConfig() {
  return useQuery<RemoteAccessConfig>({
    queryKey: CONFIG_KEY,
    queryFn: () => eveyFetch<RemoteAccessConfig>({ url: "/api/v1/admin/remote-access", method: "GET" }),
  });
}

function useNetworkInfo() {
  return useQuery<{ interfaces: NetworkInterface[] }>({
    queryKey: NETWORK_KEY,
    queryFn: () => eveyFetch<{ interfaces: NetworkInterface[] }>({ url: "/api/v1/admin/network-info", method: "GET" }),
  });
}

function RemoteAccessPage() {
  const { t } = useTranslation();
  const { data, isLoading } = useRemoteAccessConfig();
  const { data: netInfo } = useNetworkInfo();
  const queryClient = useQueryClient();

  const [enabled, setEnabled] = useState(false);
  const [externalUrl, setExternalUrl] = useState("");
  const [lanNetworks, setLanNetworks] = useState("");
  const [maxBitrate, setMaxBitrate] = useState("0");
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data) {
      setEnabled(data.enabled);
      setExternalUrl(data.externalUrl);
      setLanNetworks(data.lanNetworks.join(", "));
      setMaxBitrate(String(data.maxRemoteBitrateKbps));
    }
  }, [data]);

  const saveMutation = useMutation({
    mutationFn: (body: Partial<RemoteAccessConfig>) =>
      eveyFetch<RemoteAccessConfig>({ url: "/api/v1/admin/remote-access", method: "PUT", data: body }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CONFIG_KEY });
      toast.success(t("admin.remoteAccess.saved"));
      setDirty(false);
    },
    onError: (err: Error) => toast.error(err.message),
  });

  function handleSave() {
    const networks = lanNetworks
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    saveMutation.mutate({
      enabled,
      externalUrl,
      lanNetworks: networks,
      maxRemoteBitrateKbps: parseInt(maxBitrate, 10) || 0,
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold" style={{ color: "var(--text)" }}>
          {t("admin.remoteAccess.title")}
        </h2>
        <Badge variant={data?.enabled ? "success" : "default"}>
          {data?.enabled ? t("common.enabled") : t("common.disabled")}
        </Badge>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <div className="flex flex-col gap-6">
          <Card>
            <div className="flex flex-col gap-4 p-2">
              <div className="flex items-center gap-3">
                <label className="text-sm font-medium" style={{ color: "var(--text)" }}>
                  {t("admin.remoteAccess.enableRemoteAccess")}
                </label>
                <button
                  type="button"
                  role="switch"
                  aria-checked={enabled}
                  onClick={() => {
                    setEnabled(!enabled);
                    setDirty(true);
                  }}
                  className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors-fast"
                  style={{
                    backgroundColor: enabled ? "var(--accent)" : "var(--border)",
                  }}
                >
                  <span
                    className="inline-block h-4 w-4 rounded-full bg-white transition-transform-fast"
                    style={{
                      transform: enabled ? "translateX(1.375rem)" : "translateX(0.25rem)",
                    }}
                  />
                </button>
              </div>

              <Input
                label={t("admin.remoteAccess.externalUrl")}
                placeholder="https://evey.example.com"
                value={externalUrl}
                onChange={(e) => {
                  setExternalUrl(e.target.value);
                  setDirty(true);
                }}
              />

              <Input
                label={t("admin.remoteAccess.lanNetworks")}
                placeholder="192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12"
                value={lanNetworks}
                onChange={(e) => {
                  setLanNetworks(e.target.value);
                  setDirty(true);
                }}
              />

              <Input
                label={t("admin.remoteAccess.maxRemoteBitrate")}
                type="number"
                min={0}
                value={maxBitrate}
                onChange={(e) => {
                  setMaxBitrate(e.target.value);
                  setDirty(true);
                }}
              />

              <div className="flex justify-end pt-2">
                <Button
                  onClick={handleSave}
                  loading={saveMutation.isPending}
                  disabled={!dirty}
                >
                  {t("common.save")}
                </Button>
              </div>
            </div>
          </Card>

          {netInfo?.interfaces && netInfo.interfaces.length > 0 && (
            <Card>
              <div className="p-2">
                <h3
                  className="text-sm font-semibold mb-3"
                  style={{ color: "var(--text)" }}
                >
                  {t("admin.remoteAccess.networkInterfaces")}
                </h3>
                <div
                  className="rounded-[var(--radius-md)] overflow-hidden"
                  style={{ border: "1px solid var(--border)" }}
                >
                  <table className="w-full text-sm">
                    <thead>
                      <tr style={{ borderBottom: "1px solid var(--border)" }}>
                        <th
                          className="text-left px-4 py-2 font-medium"
                          style={{ color: "var(--text-muted)" }}
                        >
                          {t("admin.remoteAccess.interface")}
                        </th>
                        <th
                          className="text-left px-4 py-2 font-medium"
                          style={{ color: "var(--text-muted)" }}
                        >
                          {t("admin.remoteAccess.address")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {netInfo.interfaces.map((iface, i) => (
                        <tr
                          key={`${iface.name}-${i}`}
                          style={{ borderBottom: "1px solid var(--border)" }}
                        >
                          <td
                            className="px-4 py-2 font-mono text-xs"
                            style={{ color: "var(--text)" }}
                          >
                            {iface.name}
                          </td>
                          <td
                            className="px-4 py-2 font-mono text-xs"
                            style={{ color: "var(--text-muted)" }}
                          >
                            {iface.address}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </Card>
          )}
        </div>
      )}
    </div>
  );
}
