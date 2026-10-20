import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/ui/PageHeader";
import { CollapsibleCard } from "@/components/ui/CollapsibleCard";
import { SettingsGroup } from "@/components/ui/SettingsGroup";
import { EncodeCalibrationCard, EncodeSettingsCard } from "@/features/admin/EncodeCalibrationCard";
import { MaxVideoSessionsSetting, VideoSessionsCard } from "@/features/admin/transcoding/VideoSessionsCard";
import { LoadTestCard } from "@/features/admin/transcoding/LoadTestCard";
import { ConcurrencyCard } from "@/features/admin/transcoding/ConcurrencyCard";
import { AdvancedTranscodingCard } from "@/features/admin/transcoding/AdvancedTranscodingCard";
import { ServerSettingsGroup } from "@/features/admin/serverConfig/serverSettings";
import { DemoReadOnly } from "@/components/layout/DemoReadOnly";

export const Route = createLazyFileRoute("/_auth/admin/transcoding")({
  // Every control here is an operator write the demo refuses (DemoReadOnly).
  component: () => (
    <DemoReadOnly>
      <TranscodingConfigPage />
    </DemoReadOnly>
  ),
});

function SectionHeading({ id, title, desc }: { id: string; title: string; desc: string }) {
  return (
    <div className="flex flex-col gap-0.5 border-b border-[var(--border)] pb-2" data-testid={`transcoding-${id}-heading`}>
      <h2 id={`transcoding-${id}`} className="text-lg font-semibold text-[var(--text)]">{title}</h2>
      <p className="text-sm text-[var(--text-muted)]">{desc}</p>
    </div>
  );
}

// TranscodingConfigPage puts what the admin SETS apart from what the server
// MEASURED. Settings first: encoding, capacity, cache, then the expert limits
// collapsed. Diagnostics below, read-only apart from the "Recalibrate" button
// and the load test tool: the calibration readout, codec support and the
// live capacity numbers.
function TranscodingConfigPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t("admin.transcoding.title", "Transcoding configuration")}
        subtitle={t("admin.transcoding.subtitle", "Hardware acceleration, encode calibration and codec policy")}
      />

      <section aria-labelledby="transcoding-settings" className="flex flex-col gap-4" data-testid="transcoding-settings">
        <SectionHeading id="settings" title={t("admin.settingsLayout.transcodingSettings")} desc={t("admin.settingsLayout.transcodingSettingsDesc")} />

        <SettingsGroup title={t("admin.settingsLayout.groupEncoding")} description={t("admin.settingsLayout.groupEncodingDesc")} data-testid="group-encoding">
          <EncodeSettingsCard />
        </SettingsGroup>

        <SettingsGroup title={t("admin.settingsLayout.groupCapacity")} description={t("admin.settingsLayout.groupCapacityDesc")} data-testid="group-capacity">
          <MaxVideoSessionsSetting />
          <ConcurrencyCard />
        </SettingsGroup>

        <ServerSettingsGroup
          title={t("admin.settingsLayout.groupCache")}
          description={t("admin.settingsLayout.groupCacheDesc")}
          fields={["transcodeCacheMaxGB"]}
          saveTestId="server-settings-save-cache"
          data-testid="group-cache"
        />

        {/* Collapsed by default: these are expert knobs most hosts never touch,
            and most of them need a restart. */}
        <CollapsibleCard
          title={t("admin.transcoding.advancedTitle", "Advanced limits")}
          subtitle={t("admin.transcoding.advancedSubtitle", "Session and resolution ceilings for weak hardware. Applied on restart.")}
        >
          <AdvancedTranscodingCard />
        </CollapsibleCard>
      </section>

      <section aria-labelledby="transcoding-diagnostics" className="flex flex-col gap-4" data-testid="transcoding-diagnostics">
        <SectionHeading id="diagnostics" title={t("admin.settingsLayout.transcodingDiagnostics")} desc={t("admin.settingsLayout.transcodingDiagnosticsDesc")} />

        <CollapsibleCard
          title={t("admin.calibration.cardTitle", "Encode calibration")}
          subtitle={t(
            "admin.calibration.cardSubtitle",
            "Measured at boot - sets the cap on the HLS ladder so the host doesn't overshoot what it can encode in realtime.",
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
          <div className="mt-4 border-t border-[var(--border)] pt-4">
            <LoadTestCard />
          </div>
        </CollapsibleCard>
      </section>
    </div>
  );
}
