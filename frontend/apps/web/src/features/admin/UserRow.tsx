import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceDeleteUser,
  useUserServiceDisableUser,
  useUserServiceEnableUser,
  useUserServiceImpersonateUser,
  apiErrorMessage,
} from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { useNavigate } from "@tanstack/react-router";
import type { V1Profile, V1User } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { USERS_QUERY_KEY } from "./users-constants";
import { EditUserModal } from "./EditUserModal";
import { LibraryAccessModal } from "./LibraryAccessModal";
import { BitrateModal } from "./BitrateModal";
import { UserProfilesPanel } from "./UserProfilesPanel";
import { useDemoLock } from "@/hooks/useDemoMode";

// profiles are handed down from the page, which reads every profile in one
// call: fanning out one ListProfiles per row would be N+1 requests for a table
// that is almost always fully expanded on a family server.
export function UserRow({ user, profiles = [] }: { user: V1User; profiles?: V1Profile[] }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const [expanded, setExpanded] = useState(false);
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
        void navigate({ to: "/" });
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

  const toggleMutation = (doneKey: string) => ({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
        toast.success(t(doneKey));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.users.failedUpdateUser"))),
    },
  });
  // Two routes: /disable always disables, /enable re-enables.
  const disableUser = useUserServiceDisableUser(toggleMutation("admin.users.userDisabled"));
  const enableUser = useUserServiceEnableUser(toggleMutation("admin.users.userEnabled"));

  return (
    <>
    <tr className="border-b border-[var(--border)]">
      <td data-label={t("admin.users.username")} className="px-4 py-3 font-medium text-[var(--text)]">
        {user.username}
      </td>
      <td data-label={t("admin.users.profiles")} className="px-4 py-3">
        <button
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
          className="flex items-center gap-1 text-left text-sm text-[var(--text)] hover:underline"
        >
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          <span>{t("admin.users.profileCount", { count: profiles.length })}</span>
        </button>
        {!expanded && profiles.length > 0 && (
          <div className="mt-1 truncate text-xs text-[var(--text-muted)]">
            {profiles.map((p) => p.name).join(", ")}
          </div>
        )}
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
            {...lock}
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
              {...lock}
            >
              {t("admin.nav.libraries")}
            </Button>
          )}
          <Button
            variant="secondary"
            onClick={() => setShowBitrate(true)}
            {...lock}
          >
            {t("admin.users.bitrate")}
          </Button>
          <Button
            variant="secondary"
            loading={disableUser.isPending || enableUser.isPending}
            onClick={() =>
              user.enabled !== false
                ? disableUser.mutate({ id: user.id!, params: { disabled: true } })
                : enableUser.mutate({ id: user.id! })
            }
            {...lock}
          >
            {user.enabled !== false ? t("common.disable") : t("common.enable")}
          </Button>
          <Button
            variant="danger"
            loading={deleteUser.isPending}
            onClick={() => setShowDeleteConfirm(true)}
            {...lock}
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
    {expanded && (
      <tr className="border-b border-[var(--border)] bg-[var(--bg)]">
        <td colSpan={8} className="px-4 py-3">
          <UserProfilesPanel user={user} profiles={profiles} />
        </td>
      </tr>
    )}
    </>
  );
}
