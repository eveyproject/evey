import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/ui/PageHeader";
import { CollapsibleCard } from "@/components/ui/CollapsibleCard";
import { EncodeCalibrationCard } from "@/features/admin/EncodeCalibrationCard";
import { VideoSessionsCard } from "@/features/admin/transcoding/VideoSessionsCard";
import { LoadTestCard } from "@/features/admin/transcoding/LoadTestCard";
import { ConcurrencyCard } from "@/features/admin/transcoding/ConcurrencyCard";
import { AdvancedTranscodingCard } from "@/features/admin/transcoding/AdvancedTranscodingCard";
import { DemoReadOnly } from "@/components/layout/DemoReadOnly";

export const Route = createLazyFileRoute("/_auth/admin/transcoding")({
  // Every control here is an operator write the demo refuses (DemoReadOnly).
  component: () => (
    <DemoReadOnly>
      <TranscodingConfigPage />
    </DemoReadOnly>
  ),
});

// TranscodingConfigPage groups every transcoding-related admin control -
// hardware acceleration, encode calibration, AV1 and concurrency - on the
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
        <LoadTestCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.transcoding.jobConcurrencyTitle")}
        subtitle={t("admin.transcoding.jobConcurrencySubtitle")}
        alwaysOpen
      >
        <ConcurrencyCard />
      </CollapsibleCard>

      {/* Collapsed by default: these are expert knobs most hosts never touch,
          and each one needs a restart. */}
      <CollapsibleCard
        title={t("admin.transcoding.advancedTitle", "Advanced limits")}
        subtitle={t("admin.transcoding.advancedSubtitle", "Session and resolution ceilings for weak hardware. Applied on restart.")}
      >
        <AdvancedTranscodingCard />
      </CollapsibleCard>
    </div>
  );
}
