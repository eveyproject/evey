import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import type { V1User } from "@evey/api";
import { Select } from "@/components/ui/Select";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { BITRATE_OPTIONS } from "./users-constants";

export function BitrateModal({
  user,
  open,
  onClose,
}: {
  user: V1User;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [selectedBitrate, setSelectedBitrate] = useState<number>(0);
  const [saving, setSaving] = useState(false);

  const { data: currentLimit, isLoading } = useQuery<{ max_bitrate_kbps: number }>({
    queryKey: ["/api/v1/admin/users", user.id, "bitrate-limit"],
    queryFn: () =>
      eveyFetch<{ max_bitrate_kbps: number }>({
        url: `/api/v1/admin/users/${user.id}/bitrate-limit`,
        method: "GET",
      }),
    enabled: open && !!user.id,
  });

  useEffect(() => {
    if (currentLimit !== undefined) {
      setSelectedBitrate(currentLimit.max_bitrate_kbps ?? 0);
    }
  }, [currentLimit]);

  async function handleSave() {
    setSaving(true);
    try {
      await eveyFetch<void>({
        url: `/api/v1/admin/users/${user.id}/bitrate-limit`,
        method: "PATCH",
        data: { max_bitrate_kbps: selectedBitrate },
      });
      toast.success(t("admin.users.bitrateUpdated"));
      onClose();
    } catch {
      toast.error(t("admin.users.failedBitrate"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("admin.users.maxBitrateTitle", { name: user.username })}>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="md" />
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text-muted)]">
            {t("admin.users.maxBitrateDesc")}
          </p>
          <Select
            label={t("admin.users.maxBitrate")}
            value={String(selectedBitrate)}
            onChange={(v) => setSelectedBitrate(Number(v))}
            options={BITRATE_OPTIONS.map((o) => ({ label: o.label, value: String(o.value) }))}
          />
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
            <Button onClick={handleSave} loading={saving}>{t("common.save")}</Button>
          </div>
        </div>
      )}
    </Modal>
  );
}
