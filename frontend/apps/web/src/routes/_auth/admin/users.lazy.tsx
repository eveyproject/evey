import { useState, useEffect, useCallback } from "react";
import { Bell, Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useUserServiceListUsers,
  useUserServiceCreateUser,
  useUserServiceUpdateUser,
  useUserServiceDeleteUser,
  useUserServiceDisableUser,
  useUserServiceListInviteLinks,
  useUserServiceCreateInviteLink,
  useUserServiceRevokeInviteLink,
} from "@evey/api";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import type { V1User, V1InviteLink } from "@evey/api";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/users")({
  component: UsersPage,
});

const USERS_QUERY_KEY = ["/api/v1/admin/users"];
const INVITES_QUERY_KEY = ["/api/v1/invites"];

function CreateUserForm({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);

  const createUser = useUserServiceCreateUser({ mutation: { onSuccess } });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    createUser.mutate({ data: { username, email, password, isAdmin } });
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        label={t("admin.users.username")}
        value={username}
        onChange={(e) => setUsername(e.target.value)}
        required
      />
      <Input
        label={t("admin.users.email")}
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
      />
      <Input
        label={t("admin.users.password")}
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        required
      />
      <Checkbox
        label={t("admin.users.admin")}
        checked={isAdmin}
        onChange={setIsAdmin}
      />
      <div className="flex justify-end pt-2">
        <Button type="submit" loading={createUser.isPending} disabled={!username || !password}>
          {t("admin.users.createUser")}
        </Button>
      </div>
    </form>
  );
}

interface LibraryInfo {
  id: string;
  name: string;
}

interface LibraryAccessData {
  libraryIds: string[];
  libraries: LibraryInfo[];
}

function LibraryAccessModal({
  user,
  open,
  onClose,
}: {
  user: V1User;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [saving, setSaving] = useState(false);

  const { data, isLoading } = useQuery<LibraryAccessData>({
    queryKey: ["/api/v1/admin/users", user.id, "library-access"],
    queryFn: () => eveyFetch<LibraryAccessData>({ url: `/api/v1/admin/users/${user.id}/library-access`, method: "GET" }),
    enabled: open,
  });

  useEffect(() => {
    if (data) {
      setSelected(new Set(data.libraryIds));
    }
  }, [data]);

  const isUnrestricted = selected.size === 0;
  const libraries = data?.libraries ?? [];

  const handleToggle = useCallback((libId: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(libId)) {
        next.delete(libId);
      } else {
        next.add(libId);
      }
      return next;
    });
  }, []);

  async function handleSave() {
    setSaving(true);
    try {
      if (selected.size === 0) {
        await eveyFetch<void>({ url: `/api/v1/admin/users/${user.id}/library-access`, method: "DELETE" });
      } else {
        await eveyFetch<void>({
          url: `/api/v1/admin/users/${user.id}/library-access`,
          method: "PUT",
          data: { libraryIds: Array.from(selected) },
        });
      }
      toast.success(t("admin.users.libraryAccessUpdated"));
      onClose();
    } catch {
      toast.error(t("admin.users.failedLibraryAccess"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("admin.users.libraryAccessTitle", { name: user.username })}>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="md" />
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text-muted)]">
            {isUnrestricted
              ? t("admin.users.unrestrictedDesc")
              : t("admin.users.restrictedDesc", { count: selected.size, libraryWord: selected.size === 1 ? "library" : "libraries" })}
          </p>

          {isUnrestricted && (
            <Badge variant="success">{t("admin.users.unrestricted")}</Badge>
          )}

          <div className="flex flex-col gap-2">
            {libraries.map((lib) => (
              <div
                key={lib.id}
                className="px-3 py-2 rounded-[var(--radius-md)] hover:opacity-80 bg-[var(--surface)]"
              >
                <Checkbox
                  label={lib.name}
                  checked={selected.has(lib.id)}
                  onChange={() => handleToggle(lib.id)}
                />
              </div>
            ))}
          </div>

          {selected.size > 0 && (
            <Button
              variant="ghost"
              onClick={() => setSelected(new Set())}
            >
              {t("admin.users.clearAllUnrestricted")}
            </Button>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="ghost" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleSave} loading={saving}>
              {t("common.save")}
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}

const BITRATE_OPTIONS = [
  { label: "Unlimited", value: 0 },
  { label: "1 Mbps", value: 1000 },
  { label: "2 Mbps", value: 2000 },
  { label: "4 Mbps", value: 4000 },
  { label: "8 Mbps", value: 8000 },
  { label: "20 Mbps", value: 20000 },
  { label: "40 Mbps", value: 40000 },
];

function BitrateModal({
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

function EditUserModal({ user, open, onClose }: { user: V1User; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [username, setUsername] = useState(user.username ?? "");
  const [email, setEmail] = useState(user.email ?? "");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(user.isAdmin ?? false);

  useEffect(() => {
    if (open) {
      setUsername(user.username ?? "");
      setEmail(user.email ?? "");
      setPassword("");
      setIsAdmin(user.isAdmin ?? false);
    }
  }, [open, user]);

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
        toast.success(t("admin.users.userUpdated") || "User updated");
        onClose();
      },
      onError: (err: Error) => toast.error(err.message || t("admin.users.failedUpdateUser")),
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!user.id) return;
    const data: Record<string, unknown> = {};
    if (username.trim() !== user.username) data.username = username.trim();
    if (email.trim() !== (user.email ?? "")) data.email = email.trim();
    if (password) data.password = password;
    if (isAdmin !== user.isAdmin) data.isAdmin = isAdmin;
    if (Object.keys(data).length === 0) { onClose(); return; }
    updateUser.mutate({ id: user.id, data: data as any });
  }

  return (
    <Modal open={open} onClose={onClose} title={t("admin.users.editUser") || "Edit User"}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Input label={t("admin.users.username")} value={username} onChange={(e) => setUsername(e.target.value)} required />
        <Input label={t("admin.users.email")} type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        <Input label={t("admin.users.newPassword") || "New password"} type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={t("admin.users.leaveBlank") || "Leave blank to keep current"} />
        <Checkbox label={t("admin.users.admin")} checked={isAdmin} onChange={setIsAdmin} />
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
          <Button type="submit" loading={updateUser.isPending}>{t("common.save")}</Button>
        </div>
      </form>
    </Modal>
  );
}

function UserRow({ user }: { user: V1User }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showLibraryAccess, setShowLibraryAccess] = useState(false);
  const [showBitrate, setShowBitrate] = useState(false);
  const [showEdit, setShowEdit] = useState(false);

  const deleteUser = useUserServiceDeleteUser({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
        toast.success(t("admin.users.userDeleted", { name: user.username }));
      },
      onError: (err: Error) => toast.error(err.message || t("admin.users.failedDeleteUser")),
    },
  });

  const disableUser = useUserServiceDisableUser({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
        toast.success(user.enabled !== false ? t("admin.users.userDisabled") : t("admin.users.userEnabled"));
      },
      onError: (err: Error) => toast.error(err.message || t("admin.users.failedUpdateUser")),
    },
  });

  return (
    <tr className="border-b border-[var(--border)]">
      <td className="px-4 py-3 font-medium text-[var(--text)]">
        {user.username}
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text-muted)]">
        {user.email ?? "\u2014"}
      </td>
      <td className="px-4 py-3">
        {user.isAdmin ? <Badge variant="warning">{t("admin.users.admin")}</Badge> : <Badge>{t("admin.users.user")}</Badge>}
      </td>
      <td className="px-4 py-3">
        {user.enabled !== false ? (
          <Badge variant="success">{t("common.enabled")}</Badge>
        ) : (
          <Badge variant="error">{t("common.disabled")}</Badge>
        )}
      </td>
      <td className="px-4 py-3 whitespace-nowrap">
        <div className="flex gap-2 flex-wrap">
          <Button
            variant="secondary"
            onClick={() => setShowEdit(true)}
          >
            {t("common.edit") || "Edit"}
          </Button>
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

function InviteLinkRow({ link }: { link: V1InviteLink }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [showRevokeConfirm, setShowRevokeConfirm] = useState(false);

  const revoke = useUserServiceRevokeInviteLink({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: INVITES_QUERY_KEY });
        toast.success(t("admin.users.inviteRevoked"));
      },
      onError: (err: Error) => toast.error(err.message || t("admin.users.failedRevoke")),
    },
  });

  const inviteUrl = link.code ? `${window.location.origin}/register?invite=${link.code}` : "\u2014";

  function copyLink() {
    if (link.code) {
      navigator.clipboard.writeText(inviteUrl);
      toast.success(t("admin.users.linkCopied"));
    }
  }

  return (
    <tr className="border-b border-[var(--border)]">
      <td className="px-4 py-3 font-mono text-sm text-[var(--text)]">
        {link.code ?? "\u2014"}
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text-muted)]">
        {link.timesUsed ?? 0} / {link.maxUses ?? "\u221E"}
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text-muted)]">
        {link.expiresAt ? new Date(link.expiresAt).toLocaleString() : t("admin.users.never")}
      </td>
      <td className="px-4 py-3 whitespace-nowrap">
        <div className="flex gap-2">
          <Button variant="secondary" onClick={copyLink}>
            {t("admin.users.copyLink")}
          </Button>
          <Button
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

function CreateInviteForm({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useTranslation();
  const [maxUses, setMaxUses] = useState("");
  const [expiresAt, setExpiresAt] = useState("");

  const createInvite = useUserServiceCreateInviteLink({ mutation: { onSuccess } });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    createInvite.mutate({
      data: {
        maxUses: maxUses ? parseInt(maxUses) : undefined,
        expiresAt: expiresAt || undefined,
      },
    });
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        label={t("admin.users.maxUsesLabel")}
        type="number"
        min={1}
        value={maxUses}
        onChange={(e) => setMaxUses(e.target.value)}
      />
      <Input
        label={t("admin.users.expiresAtLabel")}
        type="datetime-local"
        value={expiresAt}
        onChange={(e) => setExpiresAt(e.target.value)}
      />
      <div className="flex justify-end pt-2">
        <Button type="submit" loading={createInvite.isPending}>
          {t("admin.users.createInviteLink")}
        </Button>
      </div>
    </form>
  );
}

function UsersPage() {
  const { t } = useTranslation();
  const [showUserModal, setShowUserModal] = useState(false);
  const [showInviteModal, setShowInviteModal] = useState(false);
  const queryClient = useQueryClient();
  const { data, isLoading } = useUserServiceListUsers();
  const { data: inviteData, isLoading: inviteLoading } = useUserServiceListInviteLinks();
  const users = data?.users ?? [];
  const invites = inviteData?.inviteLinks ?? [];

  function handleCreateSuccess() {
    queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
    setShowUserModal(false);
    toast.success(t("admin.users.userCreated"));
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
          <Button onClick={() => setShowUserModal(true)}>{t("admin.users.createUser")}</Button>
        </div>

        {isLoading ? (
          <div className="flex justify-center py-16">
            <Spinner size="lg" />
          </div>
        ) : (
          <div
            className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
          >
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
                  {[t("admin.users.username"), t("admin.users.email"), t("admin.users.role"), t("common.status"), t("common.actions")].map((h) => (
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
                    <td colSpan={5}>
                      <EmptyState icon={<Folder size={48} strokeWidth={1.5} />} title={t("admin.users.noUsers")} />
                    </td>
                  </tr>
                ) : (
                  users.map((u) => <UserRow key={u.id} user={u} />)
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
          <Button variant="secondary" onClick={() => setShowInviteModal(true)}>
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
            <table className="w-full text-sm">
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
                {invites.length === 0 ? (
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
