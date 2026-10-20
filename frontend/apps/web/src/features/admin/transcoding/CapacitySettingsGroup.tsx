import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { SettingsGroup } from "@/components/ui/SettingsGroup";
import { useServerSettingsForm } from "@/features/admin/serverConfig/serverSettings";
import { useMaxVideoSessionsField } from "./VideoSessionsCard";

// CapacitySettingsGroup holds the two "how many at once" limits with one
// Save: the live video streams (a config entry, it can only lower what the
// host measured) and the background conversions queued in advance (a server
// setting, a fixed default of 2 when left blank). Two subsystems, two APIs,
// one gesture for the admin.
export function CapacitySettingsGroup() {
  const { t } = useTranslation();
  const sessions = useMaxVideoSessionsField();
  const form = useServerSettingsForm(["maxConcurrentTranscodes"]);
  const [saving, setSaving] = useState(false);

  async function save() {
    setSaving(true);
    try {
      // Both run even if one fails: each reports its own outcome.
      await Promise.all([sessions.submit(), form.submit()]);
    } finally {
      setSaving(false);
    }
  }

  return (
    <SettingsGroup
      title={t("admin.settingsLayout.groupCapacity")}
      description={t("admin.settingsLayout.groupCapacityDesc")}
      data-testid="group-capacity"
      footer={
        <Button onClick={() => void save()} loading={saving} disabled={!sessions.dirty && !form.dirty} data-testid="capacity-save-btn">
          {t("common.save")}
        </Button>
      }
    >
      {sessions.row}
      {form.isLoading || !form.data ? (
        <div className="flex justify-center py-4"><Spinner size="md" /></div>
      ) : (
        form.renderField("maxConcurrentTranscodes")
      )}
    </SettingsGroup>
  );
}
