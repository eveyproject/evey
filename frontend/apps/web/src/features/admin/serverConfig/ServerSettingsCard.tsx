import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Switch } from "@/components/ui/Switch";
import { InfoTooltip } from "@/components/ui/InfoTooltip";

interface SettingValue<T> { value: T; default: T; source: "override" | "default"; }
interface ServerSettings {
  serverName: SettingValue<string>;
  hwAccelEnabled: SettingValue<boolean>;
  loginRateLimit: SettingValue<number>;
  globalRateLimit: SettingValue<number>;
  logLevel: SettingValue<string>;
  libraryWatcherEnabled: SettingValue<boolean>;
  periodicScanInterval: SettingValue<string>;
  skipPreviewEnabled: SettingValue<boolean>;
  castIntroAutoSkipEnabled: SettingValue<boolean>;
  auditRetentionDays: SettingValue<number>;
  metricsRetentionDays: SettingValue<number>;
  qoeRetentionDays: SettingValue<number>;
  playSessionRetentionDays: SettingValue<number>;
  transcodeCacheMaxGB: SettingValue<number>;
}
const SETTINGS_KEY = ["/api/v1/admin/server-settings"];
const LOG_LEVELS = ["debug", "info", "warn", "error"];

export function ServerSettingsCard() {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });
  const queryClient = useQueryClient();
  const [serverName, setServerName] = useState("");
  const [loginRate, setLoginRate] = useState("");
  const [globalRate, setGlobalRate] = useState("");
  const [logLevel, setLogLevel] = useState("");
  const [libraryWatcher, setLibraryWatcher] = useState(true);
  const [scanInterval, setScanInterval] = useState("");
  const [skipPreview, setSkipPreview] = useState(false);
  const [castIntroAutoSkip, setCastIntroAutoSkip] = useState(false);
  const [auditDays, setAuditDays] = useState("");
  const [metricsDays, setMetricsDays] = useState("");
  const [qoeDays, setQoeDays] = useState("");
  const [sessionDays, setSessionDays] = useState("");
  const [cacheGB, setCacheGB] = useState("");
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data) {
      setServerName(data.serverName.source === "override" ? String(data.serverName.value) : "");
      setLoginRate(data.loginRateLimit.source === "override" ? String(data.loginRateLimit.value) : "");
      setGlobalRate(data.globalRateLimit.source === "override" ? String(data.globalRateLimit.value) : "");
      setLogLevel(data.logLevel.source === "override" ? String(data.logLevel.value) : "");
      setLibraryWatcher(data.libraryWatcherEnabled.value);
      setScanInterval(data.periodicScanInterval.source === "override" ? String(data.periodicScanInterval.value) : "");
      setSkipPreview(data.skipPreviewEnabled.value);
      setCastIntroAutoSkip(data.castIntroAutoSkipEnabled?.value ?? false);
      setAuditDays(data.auditRetentionDays?.source === "override" ? String(data.auditRetentionDays.value) : "");
      setMetricsDays(data.metricsRetentionDays?.source === "override" ? String(data.metricsRetentionDays.value) : "");
      setQoeDays(data.qoeRetentionDays?.source === "override" ? String(data.qoeRetentionDays.value) : "");
      setSessionDays(data.playSessionRetentionDays?.source === "override" ? String(data.playSessionRetentionDays.value) : "");
      setCacheGB(data.transcodeCacheMaxGB?.source === "override" ? String(data.transcodeCacheMaxGB.value) : "");
    }
  }, [data]);

  const saveMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: SETTINGS_KEY }); toast.success(t("admin.serverSettings.saved")); setDirty(false); },
    onError: (err: Error) => toast.error(err.message),
  });

  const resetMutation = useMutation({
    mutationFn: (field: string) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: { [field]: null } }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: SETTINGS_KEY }); toast.success(t("admin.serverSettings.saved")); },
    onError: (err: Error) => toast.error(err.message),
  });

  function handleSave() {
    const body: Record<string, unknown> = {};
    if (serverName) body.serverName = serverName;
    if (loginRate) body.loginRateLimit = parseInt(loginRate, 10);
    if (globalRate) body.globalRateLimit = parseInt(globalRate, 10);
    if (logLevel) body.logLevel = logLevel;
    body.libraryWatcherEnabled = libraryWatcher;
    if (scanInterval) body.periodicScanInterval = scanInterval;
    body.skipPreviewEnabled = skipPreview;
    body.castIntroAutoSkipEnabled = castIntroAutoSkip;
    if (auditDays) body.auditRetentionDays = parseInt(auditDays, 10);
    if (metricsDays) body.metricsRetentionDays = parseInt(metricsDays, 10);
    if (qoeDays) body.qoeRetentionDays = parseInt(qoeDays, 10);
    if (sessionDays) body.playSessionRetentionDays = parseInt(sessionDays, 10);
    if (cacheGB) body.transcodeCacheMaxGB = parseInt(cacheGB, 10);
    saveMutation.mutate(body);
  }

  function ResetBtn({ setting, field }: { setting: SettingValue<unknown>; field: string }) {
    if (setting.source === "default") return <span className="text-xs text-[var(--text-muted)]">{t("admin.serverSettings.usingDefault")} {String(setting.default)}</span>;
    return <button type="button" onClick={() => resetMutation.mutate(field)} className="text-xs text-[var(--accent)] hover:underline cursor-pointer">{t("admin.serverSettings.reset")}</button>;
  }

  if (isLoading) return <div className="flex justify-center py-8"><Spinner size="md" /></div>;
  if (!data) return null;

  return (
    <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <Input id="server-name" data-testid="server-name-input" label={t("admin.serverSettings.serverName")} placeholder={data.serverName.default ?? "Evey"} value={serverName} onChange={(e) => { setServerName(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.serverName} field="serverName" />
          </div>
          <p className="text-xs text-[var(--text-muted)]">{t("admin.serverSettings.transcodeMovedHint", "Transcoding (hardware acceleration, encode calibration, AV1 and concurrency) is configured in Transcoding configuration.")}</p>
          <div className="flex flex-col gap-1">
            <Input label={t("admin.serverSettings.loginRateLimit")} type="number" min={0} placeholder={String(data.loginRateLimit.default ?? 10)} value={loginRate} onChange={(e) => { setLoginRate(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.loginRateLimit} field="loginRateLimit" />
          </div>
          <div className="flex flex-col gap-1">
            <Input label={t("admin.serverSettings.globalRateLimit")} type="number" min={0} placeholder={String(data.globalRateLimit.default ?? 300)} value={globalRate} onChange={(e) => { setGlobalRate(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.globalRateLimit} field="globalRateLimit" />
          </div>
          <div className="flex flex-col gap-1">
            <Select
              label={t("admin.serverSettings.logLevel")}
              data-testid="log-level-select"
              value={logLevel}
              onChange={(v) => { setLogLevel(v); setDirty(true); }}
              options={[{ label: `${t("admin.serverConfig.envDefault")} (${data.logLevel.default ?? "info"})`, value: "" }, ...LOG_LEVELS.map((l) => ({ label: l, value: l }))]}
            />
            <ResetBtn setting={data.logLevel} field="logLevel" />
          </div>
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <label className="text-sm font-medium text-[var(--text)]">
                {t("admin.serverSettings.libraryWatcher")} <span className="text-xs text-[var(--text-muted)]">{t("admin.serverSettings.restartHint")}</span>
              </label>
              <Switch
                checked={libraryWatcher}
                onChange={() => { setLibraryWatcher(!libraryWatcher); setDirty(true); }}
                label={t("admin.serverSettings.libraryWatcher")}
                data-testid="library-watcher-toggle"
              />
            </div>
            <ResetBtn setting={data.libraryWatcherEnabled} field="libraryWatcherEnabled" />
          </div>
          <div className="flex flex-col gap-1">
            <Input label={`${t("admin.serverSettings.periodicScanInterval")} ${t("admin.serverSettings.restartHint")}`} placeholder={String(data.periodicScanInterval.default ?? "2m")} value={scanInterval} onChange={(e) => { setScanInterval(e.target.value); setDirty(true); }} data-testid="scan-interval-input" />
            <ResetBtn setting={data.periodicScanInterval} field="periodicScanInterval" />
          </div>
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <label className="text-sm font-medium text-[var(--text)]">{t("admin.serverSettings.skipPreview")}</label>
              <InfoTooltip label={t("admin.serverSettings.skipPreview")}>
                {t("admin.serverSettings.skipPreviewInfo")}
              </InfoTooltip>
              <Switch
                checked={skipPreview}
                onChange={() => { setSkipPreview(!skipPreview); setDirty(true); }}
                label={t("admin.serverSettings.skipPreview")}
                data-testid="skip-preview-toggle"
              />
            </div>
            <ResetBtn setting={data.skipPreviewEnabled} field="skipPreviewEnabled" />
          </div>
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <label className="text-sm font-medium text-[var(--text)]">{t("admin.serverSettings.castIntroAutoSkip")}</label>
              <InfoTooltip label={t("admin.serverSettings.castIntroAutoSkip")}>
                {t("admin.serverSettings.castIntroAutoSkipInfo")}
              </InfoTooltip>
              <Switch
                checked={castIntroAutoSkip}
                onChange={() => { setCastIntroAutoSkip(!castIntroAutoSkip); setDirty(true); }}
                label={t("admin.serverSettings.castIntroAutoSkip")}
                data-testid="cast-intro-auto-skip-toggle"
              />
            </div>
            {data.castIntroAutoSkipEnabled && <ResetBtn setting={data.castIntroAutoSkipEnabled} field="castIntroAutoSkipEnabled" />}
          </div>
          {/* Retention windows are re-read by their sweeps at every pass (no
              restart); the cache budget is a boot snapshot, hence its hint. */}
          <div className="flex flex-col gap-1">
            <Input label={`${t("admin.serverSettings.auditRetentionDays")} ${t("admin.serverSettings.retentionSweepHint")}`} type="number" min={0} placeholder={String(data.auditRetentionDays?.default ?? 0)} value={auditDays} onChange={(e) => { setAuditDays(e.target.value); setDirty(true); }} data-testid="audit-retention-input" />
            {data.auditRetentionDays && <ResetBtn setting={data.auditRetentionDays} field="auditRetentionDays" />}
          </div>
          <div className="flex flex-col gap-1">
            <Input label={`${t("admin.serverSettings.metricsRetentionDays")} ${t("admin.serverSettings.retentionSweepHint")}`} type="number" min={0} placeholder={String(data.metricsRetentionDays?.default ?? 90)} value={metricsDays} onChange={(e) => { setMetricsDays(e.target.value); setDirty(true); }} data-testid="metrics-retention-input" />
            {data.metricsRetentionDays && <ResetBtn setting={data.metricsRetentionDays} field="metricsRetentionDays" />}
          </div>
          <div className="flex flex-col gap-1">
            <Input label={`${t("admin.serverSettings.qoeRetentionDays")} ${t("admin.serverSettings.retentionSweepHint")}`} type="number" min={0} placeholder={String(data.qoeRetentionDays?.default ?? 30)} value={qoeDays} onChange={(e) => { setQoeDays(e.target.value); setDirty(true); }} data-testid="qoe-retention-input" />
            {data.qoeRetentionDays && <ResetBtn setting={data.qoeRetentionDays} field="qoeRetentionDays" />}
          </div>
          <div className="flex flex-col gap-1">
            <Input label={`${t("admin.serverSettings.playSessionRetentionDays")} ${t("admin.serverSettings.retentionSweepHint")}`} type="number" min={0} placeholder={String(data.playSessionRetentionDays?.default ?? 0)} value={sessionDays} onChange={(e) => { setSessionDays(e.target.value); setDirty(true); }} data-testid="session-retention-input" />
            {data.playSessionRetentionDays && <ResetBtn setting={data.playSessionRetentionDays} field="playSessionRetentionDays" />}
          </div>
          <div className="flex flex-col gap-1">
            <Input label={`${t("admin.serverSettings.transcodeCacheMaxGB")} ${t("admin.serverSettings.restartHint")}`} type="number" min={1} placeholder={String(data.transcodeCacheMaxGB?.default ?? 50)} value={cacheGB} onChange={(e) => { setCacheGB(e.target.value); setDirty(true); }} data-testid="cache-budget-input" />
            {data.transcodeCacheMaxGB && <ResetBtn setting={data.transcodeCacheMaxGB} field="transcodeCacheMaxGB" />}
          </div>
          <div className="flex justify-end pt-2">
            <Button onClick={handleSave} loading={saveMutation.isPending} disabled={!dirty} data-testid="server-config-save-btn">{t("common.save")}</Button>
          </div>
    </div>
  );
}
