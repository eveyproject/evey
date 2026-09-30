import { useState, useEffect } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { TriangleAlert, Info } from "lucide-react";
import {
  useRemoteAccessServiceGetRemoteAccess,
  useRemoteAccessServiceGetNetworkInfo,
  remoteAccessServiceUpdateRemoteAccess,
  getRemoteAccessServiceGetRemoteAccessQueryKey,
  apiErrorMessage,
} from "@evey/api";
import type { V1UpdateRemoteAccessRequest } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Switch } from "@/components/ui/Switch";
import { useDemoLock } from "@/hooks/useDemoMode";

export const Route = createLazyFileRoute("/_auth/admin/remote-access")({
  component: RemoteAccessPage,
});

const CONFIG_KEY = getRemoteAccessServiceGetRemoteAccessQueryKey();

function RemoteAccessPage() {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  // `published`/`lockoutRisk` are part of the proto contract now, so no local
  // type extension and no cast: the generated client carries them.
  const { data, isLoading } = useRemoteAccessServiceGetRemoteAccess();
  const { data: netInfo } = useRemoteAccessServiceGetNetworkInfo();
  const queryClient = useQueryClient();

  const [enabled, setEnabled] = useState(false);
  const [externalUrl, setExternalUrl] = useState("");
  const [lanNetworks, setLanNetworks] = useState("");
  const [maxBitrate, setMaxBitrate] = useState("0");
  const [trustedProxies, setTrustedProxies] = useState("");
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data) {
      setEnabled(data.enabled ?? false);
      setExternalUrl(data.externalUrl ?? "");
      setLanNetworks((data.lanNetworks ?? []).join(", "));
      setMaxBitrate(String(data.maxRemoteBitrateKbps ?? 0));
      setTrustedProxies((data.trustedProxies ?? []).join(", "));
    }
  }, [data]);

  const saveMutation = useMutation({
    mutationFn: (body: V1UpdateRemoteAccessRequest) => remoteAccessServiceUpdateRemoteAccess(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CONFIG_KEY });
      toast.success(t("admin.remoteAccess.saved"));
      setDirty(false);
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  function handleSave() {
    const networks = lanNetworks
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    const proxies = trustedProxies
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    saveMutation.mutate({
      enabled,
      externalUrl,
      lanNetworks: networks,
      maxRemoteBitrateKbps: parseInt(maxBitrate, 10) || 0,
      // An emptied field hands the answer back to EVEY_TRUSTED_PROXIES.
      trustedProxies: proxies,
      clearTrustedProxies: proxies.length === 0,
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold text-[var(--text)]">
          {t("admin.remoteAccess.title")}
        </h2>
        <Badge variant={data?.enabled ? "success" : "default"}>
          {data?.enabled ? t("common.enabled") : t("common.disabled")}
        </Badge>
      </div>

      {!isLoading && data?.lockoutRisk && (
        <div
          role="alert"
          className="rounded-[var(--radius-md)] border border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_5%,transparent)] p-3 flex items-start gap-3"
        >
          <TriangleAlert
            size={20}
            className="text-[var(--color-danger)] flex-shrink-0 mt-0.5"
            strokeWidth={1.5}
          />
          <div className="flex flex-col gap-1">
            <p className="text-sm font-semibold text-[var(--color-danger)]">
              {t("admin.remoteAccess.lockoutRiskTitle")}
            </p>
            <p className="text-sm text-[var(--color-danger)]">
              {t("admin.remoteAccess.lockoutRiskMessage")}
            </p>
          </div>
        </div>
      )}

      {!isLoading && !data?.lockoutRisk && data?.enabled && data?.published && (
        <div
          className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] p-3 flex items-center gap-3"
        >
          <Info
            size={18}
            className="text-[var(--text-muted)] flex-shrink-0"
            strokeWidth={1.5}
          />
          <p className="text-sm text-[var(--text-muted)]">
            {t("admin.remoteAccess.publishedInfo")}
          </p>
        </div>
      )}

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <div className="flex flex-col gap-6">
          <Card>
            <div className="flex flex-col gap-4 p-2">
              <div className="flex items-center gap-3">
                <label className="text-sm font-medium text-[var(--text)]">
                  {t("admin.remoteAccess.enableRemoteAccess")}
                </label>
                <Switch
                  checked={enabled}
                  onChange={() => {
                    const next = !enabled;
                    setEnabled(next);
                    if (!next) setExternalUrl("");
                    setDirty(true);
                  }}
                  label={t("admin.remoteAccess.enableRemoteAccess")}
                />
              </div>

              <Input
                label={t("admin.remoteAccess.externalUrl")}
                placeholder={data?.serverBaseUrl || "https://evey.example.com"}
                value={externalUrl}
                onChange={(e) => {
                  setExternalUrl(e.target.value);
                  setDirty(true);
                }}
              />
              {!externalUrl && data?.serverBaseUrl && (
                <p className="text-xs text-[var(--text-muted)] -mt-2">
                  {t("admin.remoteAccess.usingDefault", "Using default:")} {data.serverBaseUrl}
                </p>
              )}
              <p className="text-xs text-[var(--text-muted)] -mt-2">
                {t("admin.remoteAccess.externalUrlHttpsHint")}
              </p>

              <Input
                label={t("admin.remoteAccess.trustedProxies")}
                placeholder="172.16.0.0/12"
                value={trustedProxies}
                onChange={(e) => {
                  setTrustedProxies(e.target.value);
                  setDirty(true);
                }}
              />
              <p className="text-xs text-[var(--text-muted)] -mt-2">
                {t("admin.remoteAccess.trustedProxiesHint")}{" "}
                {data?.trustedProxiesFromPanel
                  ? t("admin.remoteAccess.trustedProxiesFromPanel")
                  : t("admin.remoteAccess.trustedProxiesFromEnv")}
              </p>

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
                  {...lock}
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
                  className="text-sm font-semibold mb-3 text-[var(--text)]"
                >
                  {t("admin.remoteAccess.networkInterfaces")}
                </h3>
                <div
                  className="rounded-[var(--radius-md)] overflow-hidden border border-[var(--border)]"
                >
                  <table className="w-full text-sm table-cards">
                    <thead>
                      <tr className="border-b border-[var(--border)]">
                        <th
                          className="text-left px-4 py-2 font-medium text-[var(--text-muted)]"
                        >
                          {t("admin.remoteAccess.interface")}
                        </th>
                        <th
                          className="text-left px-4 py-2 font-medium text-[var(--text-muted)]"
                        >
                          {t("admin.remoteAccess.address")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {netInfo.interfaces.map((iface, i) => (
                        <tr
                          key={`${iface.name}-${i}`}
                          className="border-b border-[var(--border)]"
                        >
                          <td
                            className="px-4 py-2 font-mono text-xs text-[var(--text)]"
                            data-label={t("admin.remoteAccess.interface")}
                          >
                            {iface.name}
                          </td>
                          <td
                            className="px-4 py-2 font-mono text-xs text-[var(--text-muted)]"
                            data-label={t("admin.remoteAccess.address")}
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
