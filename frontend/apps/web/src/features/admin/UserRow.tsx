import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceDeleteUser,
  useUserServiceDisableUser,
  useUserServiceImpersonateUser,
  apiErrorMessage,
} from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { useNavigate } from "@tanstack/react-router";
import type { V1User } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { USERS_QUERY_KEY } from "./users-constants";
import { EditUserModal } from "./EditUserModal";
import { LibraryAccessModal } from "./LibraryAccessModal";
import { BitrateModal } from "./BitrateModal";

export function UserRow({ user }: { user: V1User }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showLibraryAccess, setShowLibraryAccess] = useState(false);
  const [showBitrate, setShowBitrate] = useState(false);
  const [showEdit, setShowEdit] = useState(false);

  const navigate = useNavigate();
  const doImpersonate = useAuthStore((s) => s.impersonate);
  const impersonateUser = useUserServiceImpersonateUser({
    mutation: {
      onSuccess: (data) => {
        if (!data.accessToken) return;
        // Stash the admin token, swap to the target's, land on the home grid.
        // Clear the query cache so admin-session data (which sees every library
        // unfiltered) is not served stale under the target's identity - every
        // query refetches with the impersonation token.
        doImpersonate(data.accessToken);
        queryClient.clear();
        navigate({ to: "/" });
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.users.failedImpersonate"))),
    },
  });

  const deleteUser = useUserServiceDeleteUser({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
        toast.success(t("admin.users.userDeleted", { name: user.username }));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.users.failedDeleteUser"))),
    },
  });

  const disableUser = useUserServiceDisableUser({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
        toast.success(user.enabled !== false ? t("admin.users.userDisabled") : t("admin.users.userEnabled"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.users.failedUpdateUser"))),
    },
  });

  return (
    <tr className="border-b border-[var(--border)]">
      <td data-label={t("admin.users.username")} className="px-4 py-3 font-medium text-[var(--text)]">
        {user.username}
      </td>
      <td data-label={t("admin.users.email")} className="px-4 py-3 text-sm text-[var(--text-muted)]">
        {user.email ?? "-"}
      </td>
      <td data-label={t("admin.users.role")} className="px-4 py-3">
        {user.isAdmin ? <Badge variant="warning">{t("admin.users.admin")}</Badge> : <Badge>{t("admin.users.user")}</Badge>}
      </td>
      <td data-label={t("common.status")} className="px-4 py-3">
        {user.enabled !== false ? (
          <Badge variant="success">{t("common.enabled")}</Badge>
        ) : (
          <Badge variant="error">{t("common.disabled")}</Badge>
        )}
      </td>
      <td data-label={t("admin.users.registered")} className="px-4 py-3 text-xs whitespace-nowrap text-[var(--text-muted)]">
        {user.createdAt ? new Date(user.createdAt).toLocaleString() : "-"}
      </td>
      <td data-label={t("admin.users.lastLogin")} className="px-4 py-3 text-xs whitespace-nowrap text-[var(--text-muted)]">
        {user.lastLogin ? new Date(user.lastLogin).toLocaleString() : "-"}
      </td>
      <td data-label={t("common.actions")} className="px-4 py-3 whitespace-nowrap">
        <div className="flex gap-2 flex-wrap">
          <Button
            variant="secondary"
            onClick={() => setShowEdit(true)}
          >
            {t("common.edit") || "Edit"}
          </Button>
          {user.enabled !== false && (
            <Button
              variant="secondary"
              loading={impersonateUser.isPending}
              onClick={() => impersonateUser.mutate({ userId: user.id!, data: {} })}
            >
              {t("admin.users.impersonate")}
            </Button>
          )}
          {!user.isAdmin && (
            <Button
              variant="secondary"
              onClick={() => setShowLibraryAccess(true)}
            >
              {t("admin.nav.libraries")}
            </Button>
          )}
          <Button
            variant="secondary"
            onClick={() => setShowBitrate(true)}
          >
            {t("admin.users.bitrate")}
          </Button>
          <Button
            variant="secondary"
            loading={disableUser.isPending}
            onClick={() =>
              disableUser.mutate({ id: user.id!, params: { disabled: user.enabled !== false } })
            }
          >
            {user.enabled !== false ? t("common.disable") : t("common.enable")}
          </Button>
          <Button
            variant="danger"
            loading={deleteUser.isPending}
            onClick={() => setShowDeleteConfirm(true)}
          >
            {t("common.delete")}
          </Button>
        </div>
        <Modal open={showDeleteConfirm} onClose={() => setShowDeleteConfirm(false)} title={t("admin.users.deleteUserTitle")}>
          <p className="text-sm text-[var(--text-muted)] mb-4">
            {t("admin.users.deleteUserConfirm", { name: user.username })}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowDeleteConfirm(false)}>{t("common.cancel")}</Button>
            <Button
              variant="danger"
              loading={deleteUser.isPending}
              onClick={() => {
                deleteUser.mutate({ id: user.id! });
                setShowDeleteConfirm(false);
              }}
            >
              {t("common.delete")}
            </Button>
          </div>
        </Modal>
        <EditUserModal user={user} open={showEdit} onClose={() => setShowEdit(false)} />
        {!user.isAdmin && (
          <LibraryAccessModal
            user={user}
            open={showLibraryAccess}
            onClose={() => setShowLibraryAccess(false)}
          />
        )}
        <BitrateModal
          user={user}
          open={showBitrate}
          onClose={() => setShowBitrate(false)}
        />
      </td>
    </tr>
  );
}
