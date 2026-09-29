import { useState } from "react";
import { Bell, Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useUserServiceListUsers,
  useUserServiceListInviteLinks,
  useUserServiceListAllProfiles,
} from "@evey/api";
import type { V1Profile } from "@evey/api";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { USERS_QUERY_KEY, INVITES_QUERY_KEY } from "@/features/admin/users-constants";
import { CreateUserForm } from "@/features/admin/CreateUserForm";
import { CreateInviteForm } from "@/features/admin/CreateInviteForm";
import { UserRow } from "@/features/admin/UserRow";
import { InviteLinkRow } from "@/features/admin/InviteLinkRow";
import { useDemoLock } from "@/hooks/useDemoMode";

export const Route = createLazyFileRoute("/_auth/admin/users")({
  component: UsersPage,
});

function UsersPage() {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const [showUserModal, setShowUserModal] = useState(false);
  const [showInviteModal, setShowInviteModal] = useState(false);
  const queryClient = useQueryClient();
  const { data, isLoading } = useUserServiceListUsers();
  // One read for every profile on the server, then grouped by owner. A user row
  // without its profiles cannot answer "who is behind this account?", which is
  // the whole reason this table exists.
  const { data: profileData } = useUserServiceListAllProfiles();
  const { data: inviteData, isLoading: inviteLoading, isError: inviteError, error: inviteErr, refetch: refetchInvites } = useUserServiceListInviteLinks();
  const users = data?.users ?? [];
  const invites = inviteData?.inviteLinks ?? [];
  const profilesByUser = new Map<string, V1Profile[]>();
  for (const p of profileData?.profiles ?? []) {
    if (!p.userId) continue;
    const list = profilesByUser.get(p.userId);
    if (list) list.push(p);
    else profilesByUser.set(p.userId, [p]);
  }

  function handleCreateSuccess(accessOk = true) {
    queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
    setShowUserModal(false);
    // When library-access assignment failed the form already raised an error
    // toast; don't also claim the create fully succeeded.
    if (accessOk) toast.success(t("admin.users.userCreated"));
  }

  function handleInviteSuccess() {
    queryClient.invalidateQueries({ queryKey: INVITES_QUERY_KEY });
    setShowInviteModal(false);
    toast.success(t("admin.users.inviteCreated"));
  }

  return (
    <div className="flex flex-col gap-8">
      <section className="flex flex-col gap-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold text-[var(--text)]">
            {t("admin.users.title")}
          </h2>
          <Button onClick={() => setShowUserModal(true)} {...lock}>{t("admin.users.createUser")}</Button>
        </div>

        {isLoading ? (
          <div className="flex justify-center py-16">
            <Spinner size="lg" />
          </div>
        ) : (
          <div
            className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
          >
            <table className="w-full text-sm table-cards">
              <thead>
                <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
                  {/* Profiles sit right after the username: on a family server
                      "who is behind this account" is the second thing an admin
                      reads, before the email or the role. */}
                  {[t("admin.users.username"), t("admin.users.profiles"), t("admin.users.email"), t("admin.users.role"), t("common.status"), t("admin.users.registered"), t("admin.users.lastLogin"), t("common.actions")].map((h) => (
                    <th
                      key={h}
                      className="px-4 py-2 text-left font-medium text-[var(--text-muted)]"
                    >
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {users.length === 0 ? (
                  <tr>
                    <td colSpan={8}>
                      <EmptyState icon={<Folder size={48} strokeWidth={1.5} />} title={t("admin.users.noUsers")} />
                    </td>
                  </tr>
                ) : (
                  users.map((u) => (
                    <UserRow key={u.id} user={u} profiles={profilesByUser.get(u.id ?? "") ?? []} />
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold text-[var(--text)]">
            {t("admin.users.inviteLinks")}
          </h2>
          <Button variant="secondary" onClick={() => setShowInviteModal(true)} {...lock}>
            {t("admin.users.createInviteLink")}
          </Button>
        </div>

        {inviteLoading ? (
          <div className="flex justify-center py-8">
            <Spinner size="md" />
          </div>
        ) : (
          <div
            className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
          >
            <table className="w-full text-sm table-cards">
              <thead>
                <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
                  {[t("admin.users.code"), t("admin.users.uses"), t("admin.users.expires"), t("common.actions")].map((h) => (
                    <th
                      key={h}
                      className="px-4 py-2 text-left font-medium text-[var(--text-muted)]"
                    >
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {inviteError ? (
                  // Loading is already handled by the spinner above; this is
                  // the third state, which read as "no invite links".
                  <tr>
                    <td colSpan={4}>
                      <FetchErrorState error={inviteErr} onRetry={() => void refetchInvites()} />
                    </td>
                  </tr>
                ) : invites.length === 0 ? (
                  <tr>
                    <td colSpan={4}>
                      <EmptyState icon={<Bell size={48} strokeWidth={1.5} />} title={t("admin.users.noInvites")} />
                    </td>
                  </tr>
                ) : (
                  invites.map((link) => <InviteLinkRow key={link.id} link={link} />)
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <Modal open={showUserModal} onClose={() => setShowUserModal(false)} title={t("admin.users.createUser")}>
        <CreateUserForm onSuccess={handleCreateSuccess} />
      </Modal>

      <Modal open={showInviteModal} onClose={() => setShowInviteModal(false)} title={t("admin.users.createInviteLink")}>
        <CreateInviteForm onSuccess={handleInviteSuccess} />
      </Modal>
    </div>
  );
}
