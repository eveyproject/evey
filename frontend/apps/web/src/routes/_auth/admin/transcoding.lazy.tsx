import { useEffect, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  eveyFetch,
  useAdminSystemServiceGetSystemInfo,
  useServerConfigServiceListServerConfigEntries,
  getServerConfigServiceListServerConfigEntriesQueryKey,
  serverConfigServiceSetServerConfigEntry,
} from "@evey/api";
import { toast } from "@evey/store";
import { PageHeader } from "@/components/ui/PageHeader";
import { CollapsibleCard } from "@/components/ui/CollapsibleCard";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { EncodeCalibrationCard } from "@/features/admin/EncodeCalibrationCard";

const MAX_VIDEO_SESSIONS_KEY = "transcoding.max_video_sessions";
const CONFIG_QUERY_KEY = getServerConfigServiceListServerConfigEntriesQueryKey();

// VideoSessionsCard governs the LIVE playback path: how many streams may be
// video-encoded at once. Distinct from ConcurrencyCard above, which queues the
// background transcode JOBS - two different subsystems that used to sit on two
// different admin pages and read as the same knob.
//
// The admin may only REDUCE the auto capacity the boot benchmark measured; an
// audio-only transcode (video copied) never consumes a slot, and one viewer
// switching quality holds a single slot.
function VideoSessionsCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: sysData } = useAdminSystemServiceGetSystemInfo();
  const { data: entries } = useServerConfigServiceListServerConfigEntries({
    query: { select: (d) => d.entries ?? [] },
  });

  const stored = entries?.find((e) => e.key === MAX_VIDEO_SESSIONS_KEY)?.value ?? "";
  const [value, setValue] = useState(stored);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (!dirty) setValue(stored);
  }, [stored, dirty]);

  const save = useMutation({
    mutationFn: () => serverConfigServiceSetServerConfigEntry(MAX_VIDEO_SESSIONS_KEY, { value: value.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: CONFIG_QUERY_KEY });
      setDirty(false);
      toast.success(t("admin.transcoding.videoSessionsSaved", "Video transcode capacity updated"));
    },
    onError: () => toast.error(t("admin.transcoding.videoSessionsFailed", "Could not update the capacity")),
  });

  const vs = sysData?.videoSessions;
  const ev = vs?.evidence;

  return (
    <div className="flex flex-col gap-2" data-testid="video-sessions-card">
      {vs && (
        <p className="text-sm text-[var(--text-muted)]" data-testid="video-sessions-measured">
          {t("admin.transcoding.videoSessionsMeasured", { auto: vs.autoCapacity, active: vs.active })}
        </p>
      )}
      {ev && ev.derived !== false && (
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.transcoding.videoSessionsEvidence", {
            height: ev.height,
            fps: Math.round((ev.fps ?? 0) * 10) / 10,
            requiredFps: Math.round((ev.requiredFps ?? 0) * 10) / 10,
            cores: ev.cpuBudget,
          })}
        </p>
      )}
      {ev && ev.derived === false && (
        <p className="text-xs text-[var(--warning,#b45309)]">{t("admin.transcoding.videoSessionsNotMeasured")}</p>
      )}
      <Input
        id="max-video-sessions"
        label={t("admin.transcoding.maxVideoSessionsLabel")}
        type="number"
        min={1}
        placeholder={t("admin.transcoding.maxVideoSessionsAuto")}
        value={value}
        onChange={(e) => {
          setValue(e.target.value);
          setDirty(true);
        }}
      />
      <p className="text-xs text-[var(--text-muted)]">{t("admin.transcoding.maxVideoSessionsDesc")}</p>
      <div className="flex justify-end pt-1">
        <Button
          onClick={() => save.mutate()}
          loading={save.isPending}
          disabled={!dirty}
          data-testid="max-video-sessions-save-btn"
        >
          {t("common.save")}
        </Button>
      </div>
    </div>
  );
}

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
        title={t("admin.transcoding.livePlaybackTitle")}
        subtitle={t("admin.transcoding.livePlaybackSubtitle")}
        alwaysOpen
      >
        <VideoSessionsCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.transcoding.jobConcurrencyTitle")}
        subtitle={t("admin.transcoding.jobConcurrencySubtitle")}
        alwaysOpen
      >
        <ConcurrencyCard />
      </CollapsibleCard>
    </div>
  );
}
