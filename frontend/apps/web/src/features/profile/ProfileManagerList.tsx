import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Pencil, Trash2, Star, Clock } from "lucide-react";
import {
  useUserServiceDeleteProfile,
  useUserServiceSetDefaultProfile,
  getUserServiceListProfilesQueryKey,
  apiErrorMessage,
} from "@evey/api";
import type { V1Profile } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { EditProfileDialog } from "./EditProfileDialog";
import { ProfileScheduleDialog } from "./ProfileScheduleDialog";
import { useDemoLock } from "@/hooks/useDemoMode";

interface ProfileManagerListProps {
  userId: string;
  profiles: V1Profile[];
  /**
   * Reveals the "Set as default" affordance on non-default profiles. The
   * underlying RPC is admin-only on the server, so this stays gated to the
   * admin → user-edit flow rather than the regular user settings page.
   */
  canSetDefault?: boolean;
}

export function ProfileManagerList({ userId, profiles, canSetDefault = false }: ProfileManagerListProps) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const qc = useQueryClient();
  const [editing, setEditing] = useState<V1Profile | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<V1Profile | null>(null);
  const [scheduling, setScheduling] = useState<V1Profile | null>(null);

  const hasKids = profiles.some((p) => p.isKids === true);

  const deleteProfile = useUserServiceDeleteProfile({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        toast.success(t("auth.profile.deleted", "Profile deleted"));
        setConfirmDelete(null);
      },
      onError: (err: Error) =>
        toast.error(apiErrorMessage(err, t, t("auth.profile.failedToUpdate", "Failed to update profile"))),
    },
  });

  const setDefault = useUserServiceSetDefaultProfile({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        toast.success(t("auth.profile.manage.defaultUpdated", "Default profile updated"));
      },
      onError: (err: Error) =>
        toast.error(apiErrorMessage(err, t, t("auth.profile.failedToUpdate", "Failed to update profile"))),
    },
  });

  return (
    <>
      <ul className="flex flex-col gap-2">
        {profiles.map((p) => {
          const initial = (p.name ?? "?").charAt(0).toUpperCase();
          const avatarUrl = p.avatarPath && p.id ? `/api/v1/profiles/${p.id}/avatar` : null;
          const isDefault = p.isDefault === true;
          return (
            <li
              key={p.id}
              className="flex items-center gap-3 p-3 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)]"
            >
              <div className="w-10 h-10 rounded-full bg-[var(--accent)] text-[var(--filter-active-text)] flex items-center justify-center font-semibold overflow-hidden shrink-0">
                {avatarUrl ? (
                  <img src={avatarUrl} alt="" className="w-full h-full object-cover" />
                ) : (
                  initial
                )}
              </div>
              <div className="flex-1 min-w-0 flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-[var(--text)] truncate">{p.name}</span>
                {isDefault && (
                  <Badge variant="info">{t("auth.profile.manage.defaultProfileBadge", "Default")}</Badge>
                )}
                {p.isKids && (
                  <Badge variant="success">{t("auth.profile.manage.kidsBadge", "Kids")}</Badge>
                )}
              </div>
              {canSetDefault && !isDefault && (
                <Button
                  {...lock}
                  variant="ghost"
                  aria-label={t("auth.profile.manage.setDefault", "Set as default")}
                  loading={setDefault.isPending && setDefault.variables?.id === p.id}
                  onClick={() => {
                    if (p.id) setDefault.mutate({ id: p.id, data: {} });
                  }}
                >
                  <Star size={16} aria-hidden="true" />
                </Button>
              )}
              {p.isKids && (
                <Button
                  variant="ghost"
                  aria-label={t("auth.profile.schedule.title", "Opening hours")}
                  title={t("auth.profile.schedule.title", "Opening hours")}
                  onClick={() => setScheduling(p)}
                >
                  <Clock size={16} aria-hidden="true" />
                </Button>
              )}
              <Button
                {...lock}
                variant="ghost"
                aria-label={t("common.edit", "Edit")}
                onClick={() => setEditing(p)}
              >
                <Pencil size={16} aria-hidden="true" />
              </Button>
              <span
                title={
                  isDefault
                    ? t(
                        "auth.profile.manage.cannotDeleteDefault",
                        "Default profile cannot be deleted.",
                      )
                    : undefined
                }
              >
                <Button
                  {...lock}
                  variant="danger"
                  aria-label={t("common.delete", "Delete")}
                  disabled={!!lock.disabled || (isDefault)}
                  onClick={() => setConfirmDelete(p)}
                >
                  <Trash2 size={16} aria-hidden="true" />
                </Button>
              </span>
            </li>
          );
        })}
      </ul>

      {editing && (
        <EditProfileDialog
          open={!!editing}
          profile={editing}
          hasKidsProfile={hasKids}
          userId={userId}
          onClose={() => setEditing(null)}
        />
      )}

      {scheduling?.id && (
        <ProfileScheduleDialog
          open
          profileId={scheduling.id}
          profileName={scheduling.name ?? ""}
          onClose={() => setScheduling(null)}
        />
      )}

      <Modal
        open={!!confirmDelete}
        onClose={() => setConfirmDelete(null)}
        title={t("common.delete", "Delete")}
      >
        <p className="text-sm text-[var(--text-muted)] mb-6">
          {t("auth.profile.manage.deleteConfirm", 'Delete profile "{{name}}"? This cannot be undone.', {
            name: confirmDelete?.name ?? "",
          })}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setConfirmDelete(null)}>
            {t("common.cancel", "Cancel")}
          </Button>
          <Button
            {...lock}
            variant="danger"
            loading={deleteProfile.isPending}
            onClick={() => {
              if (confirmDelete?.id) deleteProfile.mutate({ id: confirmDelete.id });
            }}
          >
            {t("common.delete", "Delete")}
          </Button>
        </div>
      </Modal>
    </>
  );
}
