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

export const Route = createLazyFileRoute("/_auth/admin/server-settings")({
  component: ServerSettingsPage,
});

interface SettingValue<T> {
  value: T;
  default: T;
  source: "override" | "default";
}

interface ServerSettings {
  serverName: SettingValue<string>;
  maxConcurrentTranscodes: SettingValue<number>;
  hwAccelEnabled: SettingValue<boolean>;
  loginRateLimit: SettingValue<number>;
  globalRateLimit: SettingValue<number>;
}

const SETTINGS_KEY = ["/api/v1/admin/server-settings"];

function useServerSettings() {
  return useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () =>
      eveyFetch<ServerSettings>({
        url: "/api/v1/admin/server-settings",
        method: "GET",
      }),
  });
}

function ServerSettingsPage() {
  const { t } = useTranslation();
  const { data, isLoading } = useServerSettings();
  const queryClient = useQueryClient();

  const [serverName, setServerName] = useState("");
  const [maxTranscodes, setMaxTranscodes] = useState("");
  const [hwAccel, setHwAccel] = useState(false);
  const [loginRate, setLoginRate] = useState("");
  const [globalRate, setGlobalRate] = useState("");
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data) {
      setServerName(
        data.serverName.source === "override"
          ? String(data.serverName.value)
          : "",
      );
      setMaxTranscodes(
        data.maxConcurrentTranscodes.source === "override"
          ? String(data.maxConcurrentTranscodes.value)
          : "",
      );
      setHwAccel(data.hwAccelEnabled.value);
      setLoginRate(
        data.loginRateLimit.source === "override"
          ? String(data.loginRateLimit.value)
          : "",
      );
      setGlobalRate(
        data.globalRateLimit.source === "override"
          ? String(data.globalRateLimit.value)
          : "",
      );
    }
  }, [data]);

  const saveMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({
        url: "/api/v1/admin/server-settings",
        method: "PUT",
        data: body,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: SETTINGS_KEY });
      toast.success(t("admin.serverSettings.saved"));
      setDirty(false);
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const resetMutation = useMutation({
    mutationFn: (field: string) =>
      eveyFetch<ServerSettings>({
        url: "/api/v1/admin/server-settings",
        method: "PUT",
        data: { [field]: null },
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: SETTINGS_KEY });
      toast.success(t("admin.serverSettings.saved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  function handleSave() {
    const body: Record<string, unknown> = {};
    if (serverName) body.serverName = serverName;
    if (maxTranscodes) body.maxConcurrentTranscodes = parseInt(maxTranscodes, 10);
    body.hwAccelEnabled = hwAccel;
    if (loginRate) body.loginRateLimit = parseInt(loginRate, 10);
    if (globalRate) body.globalRateLimit = parseInt(globalRate, 10);
    saveMutation.mutate(body);
  }

  function handleReset(field: string) {
    resetMutation.mutate(field);
  }

  function DefaultHint({ setting }: { setting: SettingValue<unknown> }) {
    if (setting.source === "default") {
      return (
        <span className="text-xs text-[var(--text-muted)]">
          {t("admin.serverSettings.usingDefault")} {String(setting.default)}
        </span>
      );
    }
    return (
      <button
        type="button"
        onClick={() => {
          const key = Object.entries(data ?? {}).find(
            ([, v]) => v === setting,
          )?.[0];
          if (key) handleReset(key);
        }}
        className="text-xs text-[var(--accent)] hover:underline cursor-pointer"
      >
        {t("admin.serverSettings.reset")}
      </button>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-xl font-semibold text-[var(--text)]">
        {t("admin.serverSettings.title")}
      </h2>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <Card>
          <div className="flex flex-col gap-4 p-2">
            {/* Server Name */}
            <div className="flex flex-col gap-1">
              <Input
                label={t("admin.serverSettings.serverName")}
                placeholder={data?.serverName.default ?? "Evey"}
                value={serverName}
                onChange={(e) => {
                  setServerName(e.target.value);
                  setDirty(true);
                }}
              />
              {data && <DefaultHint setting={data.serverName} />}
            </div>

            {/* Max Concurrent Transcodes */}
            <div className="flex flex-col gap-1">
              <Input
                label={t("admin.serverSettings.maxConcurrentTranscodes")}
                type="number"
                min={0}
                placeholder={String(data?.maxConcurrentTranscodes.default ?? 2)}
                value={maxTranscodes}
                onChange={(e) => {
                  setMaxTranscodes(e.target.value);
                  setDirty(true);
                }}
              />
              {data && <DefaultHint setting={data.maxConcurrentTranscodes} />}
            </div>

            {/* Hardware Acceleration */}
            <div className="flex flex-col gap-1">
              <div className="flex items-center gap-3">
                <label className="text-sm font-medium text-[var(--text)]">
                  {t("admin.serverSettings.hwAccelEnabled")}
                </label>
                <button
                  type="button"
                  role="switch"
                  aria-checked={hwAccel}
                  onClick={() => {
                    setHwAccel(!hwAccel);
                    setDirty(true);
                  }}
                  className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors-fast"
                  style={{
                    backgroundColor: hwAccel
                      ? "var(--accent)"
                      : "var(--border)",
                  }}
                >
                  <span
                    className="inline-block h-4 w-4 rounded-full bg-white transition-transform-fast"
                    style={{
                      transform: hwAccel
                        ? "translateX(1.375rem)"
                        : "translateX(0.25rem)",
                    }}
                  />
                </button>
              </div>
              {data && <DefaultHint setting={data.hwAccelEnabled} />}
            </div>

            {/* Login Rate Limit */}
            <div className="flex flex-col gap-1">
              <Input
                label={t("admin.serverSettings.loginRateLimit")}
                type="number"
                min={0}
                placeholder={String(data?.loginRateLimit.default ?? 10)}
                value={loginRate}
                onChange={(e) => {
                  setLoginRate(e.target.value);
                  setDirty(true);
                }}
              />
              {data && <DefaultHint setting={data.loginRateLimit} />}
            </div>

            {/* Global Rate Limit */}
            <div className="flex flex-col gap-1">
              <Input
                label={t("admin.serverSettings.globalRateLimit")}
                type="number"
                min={0}
                placeholder={String(data?.globalRateLimit.default ?? 300)}
                value={globalRate}
                onChange={(e) => {
                  setGlobalRate(e.target.value);
                  setDirty(true);
                }}
              />
              {data && <DefaultHint setting={data.globalRateLimit} />}
            </div>

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
      )}
    </div>
  );
}
