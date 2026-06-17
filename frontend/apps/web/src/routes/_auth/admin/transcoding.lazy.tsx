import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/ui/PageHeader";
import { CollapsibleCard } from "@/components/ui/CollapsibleCard";
import { EncodeCalibrationCard } from "@/features/admin/EncodeCalibrationCard";

export const Route = createLazyFileRoute("/_auth/admin/transcoding")({
  component: TranscodingConfigPage,
});

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
    </div>
  );
}
