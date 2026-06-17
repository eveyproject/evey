import { useEffect, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { PageHeader } from "@/components/ui/PageHeader";
import { CollapsibleCard } from "@/components/ui/CollapsibleCard";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { EncodeCalibrationCard } from "@/features/admin/EncodeCalibrationCard";

export const Route = createLazyFileRoute("/_auth/admin/transcoding")({
  component: TranscodingConfigPage,
});

const SETTINGS_KEY = ["/api/v1/admin/server-settings"];

type SettingValue<T> = { value: T; default: T; source: string };
interface ServerSettings {
  maxConcurrentTranscodes: SettingValue<number>;
}

// ConcurrencyCard owns the max-simultaneous-transcodes policy: Auto (no
// override → the server's own default) or a manual cap. A weak NAS that can
// barely sustain one 4K transcode sets a low manual number; most hosts leave
// it on Auto.
function ConcurrencyCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading } = useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });

  const [auto, setAuto] = useState(true);
  const [manual, setManual] = useState("");
  useEffect(() => {
    if (data) {
      const overridden = data.maxConcurrentTranscodes.source === "override";
      setAuto(!overridden);
      setManual(overridden ? String(data.maxConcurrentTranscodes.value) : "");
    }
  }, [data]);

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: SETTINGS_KEY }); toast.success(t("admin.transcoding.saved", "Saved")); },
    onError: (err: Error) => toast.error(err.message),
  });

  if (isLoading || !data) return <div className="flex justify-center py-6"><Spinner size="md" /></div>;

  const defaultVal = data.maxConcurrentTranscodes.default;

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <label className="text-sm font-medium text-[var(--text)]">
          {t("admin.transcoding.concurrencyAuto", "Detect automatically")}
        </label>
        <button
          type="button"
          role="switch"
          aria-checked={auto}
          aria-label={t("admin.transcoding.concurrencyAuto", "Detect automatically")}
          data-testid="max-concurrent-auto-toggle"
          onClick={() => setAuto((v) => !v)}
          className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors"
          style={{ backgroundColor: auto ? "var(--accent)" : "var(--border)" }}
        >
          <span className="inline-block h-4 w-4 rounded-full bg-white transition-transform"
            style={{ transform: auto ? "translateX(1.375rem)" : "translateX(0.25rem)" }} />
        </button>
        <span className="text-xs text-[var(--text-muted)]">
          {auto
            ? t("admin.transcoding.concurrencyAutoHint", "Server default ({{n}})", { n: defaultVal })
            : t("admin.transcoding.concurrencyManualHint", "Cap concurrent transcodes")}
        </span>
      </div>

      {!auto && (
        <Input
          label={t("admin.serverSettings.maxConcurrentTranscodes", "Max simultaneous transcodes")}
          type="number"
          min={1}
          placeholder={String(defaultVal)}
          value={manual}
          onChange={(e) => setManual(e.target.value)}
          data-testid="max-concurrent-input"
        />
      )}

      <div className="flex justify-end">
        <Button
          data-testid="concurrency-save-btn"
          loading={save.isPending}
          onClick={() => {
            // Auto → clear the override (server falls back to its default).
            // Manual → persist the chosen cap.
            save.mutate({ maxConcurrentTranscodes: auto || manual === "" ? null : parseInt(manual, 10) });
          }}
        >
          {t("common.save")}
        </Button>
      </div>
    </div>
  );
}

// TranscodingConfigPage groups every transcoding-related admin control —
// hardware acceleration, encode calibration, AV1 and concurrency — on the
// server-config side of the admin (a sibling of "Server configuration"),
// out of the Media submenu where it used to live.
function TranscodingConfigPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t("admin.transcoding.title", "Transcoding configuration")}
        subtitle={t("admin.transcoding.subtitle", "Hardware acceleration, encode calibration and codec policy")}
      />

      <CollapsibleCard
        title={t("admin.calibration.cardTitle", "Encode calibration")}
        subtitle={t(
          "admin.calibration.cardSubtitle",
          "Measured at boot — sets the cap on the HLS ladder so the host doesn't overshoot what it can encode in realtime.",
        )}
        alwaysOpen
      >
        <EncodeCalibrationCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.transcoding.concurrencyTitle", "Concurrency")}
        subtitle={t("admin.transcoding.concurrencySubtitle", "How many transcodes may run at once")}
        alwaysOpen
      >
        <ConcurrencyCard />
      </CollapsibleCard>
    </div>
  );
}
