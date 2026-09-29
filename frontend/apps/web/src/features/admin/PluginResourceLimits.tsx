import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceGetPluginResourceLimits,
  useAdminPluginServiceUpdatePluginResourceLimits,
  getAdminPluginServiceGetPluginResourceLimitsQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { useDemoLock } from "@/hooks/useDemoMode";

const SANDBOX_MODES = ["auto", "landlock", "bubblewrap", "strict", "basic"];

/**
 * Admin form for the per-plugin resource caps the runtime's resource monitor
 * enforces. Memory/PIDs/disk apply on the next sweep; cpu nice and sandbox mode
 * apply to plugins (re)started afterwards.
 */
export function PluginResourceLimits() {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const { data, isLoading } = useAdminPluginServiceGetPluginResourceLimits();

  const [memoryMb, setMemoryMb] = useState(0);
  const [pidsMax, setPidsMax] = useState(0);
  const [diskMaxMb, setDiskMaxMb] = useState(0);
  const [cpuNice, setCpuNice] = useState(0);
  const [sandboxMode, setSandboxMode] = useState("auto");

  useEffect(() => {
    if (!data) return;
    setMemoryMb(data.memoryMb ?? 0);
    setPidsMax(data.pidsMax ?? 0);
    setDiskMaxMb(data.diskMaxMb ?? 0);
    setCpuNice(data.cpuNice ?? 0);
    setSandboxMode(data.sandboxMode || "auto");
  }, [data]);

  const update = useAdminPluginServiceUpdatePluginResourceLimits({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({
          queryKey: getAdminPluginServiceGetPluginResourceLimitsQueryKey(),
        });
        toast.success(t("admin.plugins.resourceLimits.saved"));
      },
      onError: () => toast.error(t("admin.plugins.resourceLimits.saveFailed")),
    },
  });

  function handleSave() {
    update.mutate({ data: { memoryMb, pidsMax, diskMaxMb, cpuNice, sandboxMode } });
  }

  return (
    <Card data-testid="plugin-resource-limits">
      <div className="flex flex-col gap-4 p-4">
        <div className="flex flex-col gap-1">
          <span className="font-semibold text-[var(--text)]">
            {t("admin.plugins.resourceLimits.title")}
          </span>
          <span className="text-xs text-[var(--text-muted)]">
            {t("admin.plugins.resourceLimits.description")}
          </span>
        </div>

        {isLoading ? (
          <div className="flex justify-center py-6">
            <Spinner />
          </div>
        ) : (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Input
                label={t("admin.plugins.resourceLimits.memoryMb")}
                type="number"
                min={0}
                value={memoryMb}
                onChange={(e) => setMemoryMb(Number(e.target.value))}
                data-testid="rl-memory"
              />
              <Input
                label={t("admin.plugins.resourceLimits.pidsMax")}
                type="number"
                min={0}
                value={pidsMax}
                onChange={(e) => setPidsMax(Number(e.target.value))}
                data-testid="rl-pids"
              />
              <Input
                label={t("admin.plugins.resourceLimits.diskMaxMb")}
                type="number"
                min={0}
                value={diskMaxMb}
                onChange={(e) => setDiskMaxMb(Number(e.target.value))}
                data-testid="rl-disk"
              />
              <Input
                label={t("admin.plugins.resourceLimits.cpuNice")}
                type="number"
                min={0}
                max={19}
                value={cpuNice}
                onChange={(e) => setCpuNice(Number(e.target.value))}
                data-testid="rl-nice"
              />
              <Select
                label={t("admin.plugins.resourceLimits.sandboxMode")}
                value={sandboxMode}
                onChange={setSandboxMode}
                options={SANDBOX_MODES.map((m) => ({ label: m, value: m }))}
                data-testid="rl-sandbox"
              />
            </div>
            <div className="flex justify-end">
              <Button onClick={handleSave} loading={update.isPending} data-testid="rl-save" {...lock}>
                {t("admin.plugins.resourceLimits.save")}
              </Button>
            </div>
          </>
        )}
      </div>
    </Card>
  );
}
