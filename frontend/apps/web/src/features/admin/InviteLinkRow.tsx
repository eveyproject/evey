import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceRevokeInviteLink, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import type { V1InviteLink } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { INVITES_QUERY_KEY } from "./users-constants";
import { useDemoLock } from "@/hooks/useDemoMode";

export function InviteLinkRow({ link }: { link: V1InviteLink }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const [showRevokeConfirm, setShowRevokeConfirm] = useState(false);

  const revoke = useUserServiceRevokeInviteLink({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: INVITES_QUERY_KEY });
        toast.success(t("admin.users.inviteRevoked"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.users.failedRevoke"))),
    },
  });

  const inviteUrl = link.code ? `${window.location.origin}/register?invite=${link.code}` : "-";

  async function copyLink() {
    if (link.code) {
      try {
        await navigator.clipboard.writeText(inviteUrl);
        toast.success(t("admin.users.linkCopied"));
      } catch {
        toast.error(t("common.clipboardFailed"));
      }
    }
  }

  return (
    <tr className="border-b border-[var(--border)]">
      <td data-label={t("admin.users.code")} className="px-4 py-3 font-mono text-sm text-[var(--text)]">
        {link.code ?? "-"}
      </td>
      <td data-label={t("admin.users.uses")} className="px-4 py-3 text-sm text-[var(--text-muted)]">
        {link.timesUsed ?? 0} / {link.maxUses ?? "\u221E"}
        {link.maxProfiles != null && (
          <div className="text-xs">{t("admin.users.inviteMaxProfiles", { max: link.maxProfiles })}</div>
        )}
      </td>
      <td data-label={t("admin.users.expires")} className="px-4 py-3 text-sm text-[var(--text-muted)]">
        {link.expiresAt ? new Date(link.expiresAt).toLocaleString() : t("admin.users.never")}
      </td>
      <td data-label={t("common.actions")} className="px-4 py-3 whitespace-nowrap">
        <div className="flex gap-2">
          <Button variant="secondary" onClick={copyLink}>
            {t("admin.users.copyLink")}
          </Button>
          <Button
            {...lock}
            variant="danger"
            loading={revoke.isPending}
            onClick={() => setShowRevokeConfirm(true)}
          >
            {t("admin.users.revoke")}
          </Button>
        </div>
        <Modal open={showRevokeConfirm} onClose={() => setShowRevokeConfirm(false)} title={t("admin.users.revokeInviteTitle")}>
          <p className="text-sm text-[var(--text-muted)] mb-4">
            {t("admin.users.revokeInviteConfirm")}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowRevokeConfirm(false)}>{t("common.cancel")}</Button>
            <Button
              {...lock}
              variant="danger"
              loading={revoke.isPending}
              onClick={() => {
                revoke.mutate({ id: link.id! });
                setShowRevokeConfirm(false);
              }}
            >
              {t("admin.users.revoke")}
            </Button>
          </div>
        </Modal>
      </td>
    </tr>
  );
}
