import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import { useUserServiceUpdateUser } from "@evey/api";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import type { V1User } from "@evey/api";
import { Checkbox } from "@/components/ui/Checkbox";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { USERS_QUERY_KEY, type LibraryAccessData } from "./users-constants";

export function EditUserModal({ user, open, onClose }: { user: V1User; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [username, setUsername] = useState(user.username ?? "");
  const [email, setEmail] = useState(user.email ?? "");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(user.isAdmin ?? false);
  const [selectedLibs, setSelectedLibs] = useState<Set<string>>(new Set());
  const [saving, setSaving] = useState(false);

  const { data: accessData, isLoading: accessLoading } = useQuery<LibraryAccessData>({
    queryKey: ["/api/v1/admin/users", user.id, "library-access"],
    queryFn: () => eveyFetch<LibraryAccessData>({ url: `/api/v1/admin/users/${user.id}/library-access`, method: "GET" }),
    enabled: open && !isAdmin,
  });

  const libraries = accessData?.libraries ?? [];

  useEffect(() => {
    if (open) {
      setUsername(user.username ?? "");
      setEmail(user.email ?? "");
      setPassword("");
      setIsAdmin(user.isAdmin ?? false);
    }
  }, [open, user]);

  useEffect(() => {
    if (accessData) {
      setSelectedLibs(new Set(accessData.libraryIds));
    }
  }, [accessData]);

  const handleToggleLib = useCallback((id: string) => {
    setSelectedLibs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }, []);

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onError: (err: Error) => toast.error(err.message || t("admin.users.failedUpdateUser")),
    },
  });

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!user.id) return;
    setSaving(true);

    try {
      const data: Record<string, unknown> = {};
      if (username.trim() !== user.username) data.username = username.trim();
      if (email.trim() !== (user.email ?? "")) data.email = email.trim();
      if (password) data.password = password;
      if (isAdmin !== user.isAdmin) data.isAdmin = isAdmin;

      if (Object.keys(data).length > 0) {
        await updateUser.mutateAsync({ id: user.id, data: data as any });
      }

      if (!isAdmin) {
        const origIds = new Set(accessData?.libraryIds ?? []);
        const changed = selectedLibs.size !== origIds.size || [...selectedLibs].some((id) => !origIds.has(id));
        if (changed) {
          if (selectedLibs.size === 0) {
            await eveyFetch<void>({ url: `/api/v1/admin/users/${user.id}/library-access`, method: "DELETE" });
          } else {
            await eveyFetch<void>({
              url: `/api/v1/admin/users/${user.id}/library-access`,
              method: "PUT",
              data: { libraryIds: Array.from(selectedLibs) },
            });
          }
        }
      }

      queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
      queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/users", user.id, "library-access"] });
      toast.success(t("admin.users.userUpdated") || "User updated");
      onClose();
    } catch (err: any) {
      toast.error(err?.message || t("admin.users.failedUpdateUser"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("admin.users.editUser") || "Edit User"}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Input label={t("admin.users.username")} value={username} onChange={(e) => setUsername(e.target.value)} required />
        <Input label={t("admin.users.email")} type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        <Input label={t("admin.users.newPassword") || "New password"} type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={t("admin.users.leaveBlank") || "Leave blank to keep current"} />
        <Checkbox label={t("admin.users.admin")} checked={isAdmin} onChange={setIsAdmin} />
        {!isAdmin && (
          accessLoading ? (
            <div className="flex justify-center py-4">
              <Spinner size="sm" />
            </div>
          ) : libraries.length > 0 ? (
            <div className="flex flex-col gap-2">
              <p className="text-sm font-medium text-[var(--text)]">
                {t("admin.users.libraryAccessTitle", { name: "" }).replace(/\s*$/, "")}
              </p>
              <p className="text-sm text-[var(--text-muted)]">
                {selectedLibs.size === 0
                  ? t("admin.users.unrestrictedDesc")
                  : t("admin.users.restrictedDesc", { count: selectedLibs.size, libraryWord: selectedLibs.size === 1 ? "library" : "libraries" })}
              </p>
              {selectedLibs.size === 0 && (
                <Badge variant="success">{t("admin.users.unrestricted")}</Badge>
              )}
              <div className="flex flex-col gap-2">
                {libraries.map((lib) => (
                  <div key={lib.id} className="px-3 py-2 rounded-[var(--radius-md)] hover:opacity-80 bg-[var(--surface)]">
                    <Checkbox
                      label={lib.name}
                      checked={selectedLibs.has(lib.id)}
                      onChange={() => handleToggleLib(lib.id)}
                    />
                  </div>
                ))}
              </div>
              {selectedLibs.size > 0 && (
                <Button variant="ghost" onClick={() => setSelectedLibs(new Set())} type="button">
                  {t("admin.users.clearAllUnrestricted")}
                </Button>
              )}
            </div>
          ) : null
        )}
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
          <Button type="submit" loading={saving}>{t("common.save")}</Button>
        </div>
      </form>
    </Modal>
  );
}
