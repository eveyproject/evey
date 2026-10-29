import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceUpdateUser,
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  useLibraryAccessServiceGetUserLibraryAccess,
  useLibraryAccessServiceRevokeAllUserLibraryAccess,
  useLibraryAccessServiceSetUserLibraryAccess,
  getLibraryAccessServiceGetUserLibraryAccessQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1User, UserServiceUpdateUserBody } from "@evey/api";
import { Checkbox } from "@/components/ui/Checkbox";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { SegmentedControl } from "@/components/ui/SegmentedControl";
import { Input } from "@/components/ui/Input";
import { ProfileManagerList } from "@/features/profile/ProfileManagerList";
import { CreateProfileDialog } from "@/features/profile/CreateProfileDialog";
import { USERS_QUERY_KEY } from "./users-constants";

export function EditUserModal({ user, open, onClose }: { user: V1User; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [username, setUsername] = useState(user.username ?? "");
  const [email, setEmail] = useState(user.email ?? "");
  const [password, setPassword] = useState("");
  const [currentPwd, setCurrentPwd] = useState("");
  // An admin editing their OWN account re-enters their password, like on the
  // settings page; editing someone else needs no proof (AUTH-3).
  const { data: me } = useUserServiceGetCurrentUser();
  const isSelf = !!me?.id && me.id === user.id;
  const [isAdmin, setIsAdmin] = useState(user.isAdmin ?? false);
  const [maxProfiles, setMaxProfiles] = useState<string>(String(user.maxProfiles ?? 0));
  const [maxDevicesPerProfile, setMaxDevicesPerProfile] = useState<string>(String(user.maxDevicesPerProfile ?? 0));
  const [selectedLibs, setSelectedLibs] = useState<Set<string>>(new Set());
  // "All libraries" and "no library ticked" are two different answers, so the
  // admin picks the mode explicitly: an empty selection used to be saved as a
  // revoke-all, which lifts the restriction and opens the whole catalogue.
  const [libMode, setLibMode] = useState<"all" | "selected">("all");
  const [saving, setSaving] = useState(false);
  const [creatingProfile, setCreatingProfile] = useState(false);

  const { data: accessData, isLoading: accessLoading } = useLibraryAccessServiceGetUserLibraryAccess(
    user.id ?? "",
    { query: { enabled: open && !isAdmin && !!user.id } },
  );

  const { data: profilesData, isLoading: profilesLoading } = useUserServiceListProfiles(
    user.id ?? "",
    { query: { enabled: open && !!user.id } },
  );
  const profiles = profilesData?.profiles ?? [];

  const libraries = accessData?.libraries ?? [];

  useEffect(() => {
    if (open) {
      setUsername(user.username ?? "");
      setEmail(user.email ?? "");
      setPassword("");
      setCurrentPwd("");
      setIsAdmin(user.isAdmin ?? false);
      setMaxProfiles(String(user.maxProfiles ?? 0));
      setMaxDevicesPerProfile(String(user.maxDevicesPerProfile ?? 0));
      // Clear stale library selection from a previously-edited user; the
      // accessData effect repopulates it once this user's access loads.
      setSelectedLibs(new Set());
      setLibMode("all");
    }
    // Key on user.id, not the whole object: `user` is a list-query row whose
    // identity changes on any users-list refetch, and depending on it would wipe
    // in-progress edits (including a typed password) mid-edit. We only want to
    // reset when the modal opens or switches to a different user.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, user.id]);

  useEffect(() => {
    if (accessData) {
      setSelectedLibs(new Set(accessData.libraryIds ?? []));
      setLibMode(accessData.restricted || (accessData.libraryIds?.length ?? 0) > 0 ? "selected" : "all");
    }
  }, [accessData]);

  const handleToggleLib = useCallback((id: string) => {
    setSelectedLibs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }, []);

  // No onError here - the catch block in handleSubmit is the single source of
  // truth for error toasts. A dual onError+catch pattern would show two toasts.
  const updateUser = useUserServiceUpdateUser();
  const revokeAccess = useLibraryAccessServiceRevokeAllUserLibraryAccess();
  const setAccess = useLibraryAccessServiceSetUserLibraryAccess();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!user.id) return;
    const credentialChange = username.trim() !== user.username || email.trim() !== (user.email ?? "") || !!password;
    if (isSelf && credentialChange && !currentPwd) {
      toast.error(t("errors.user.current_password_required"));
      return;
    }
    setSaving(true);

    try {
      const data: UserServiceUpdateUserBody = {};
      if (username.trim() !== user.username) data.username = username.trim();
      if (email.trim() !== (user.email ?? "")) data.email = email.trim();
      if (password) data.password = password;
      if (isAdmin !== user.isAdmin) data.isAdmin = isAdmin;
      const parsedMaxProfiles = Number.isFinite(Number(maxProfiles)) ? Math.max(0, Math.trunc(Number(maxProfiles))) : 0;
      const parsedMaxDevices = Number.isFinite(Number(maxDevicesPerProfile)) ? Math.max(0, Math.trunc(Number(maxDevicesPerProfile))) : 0;
      if (parsedMaxProfiles !== (user.maxProfiles ?? 0)) data.maxProfiles = parsedMaxProfiles;
      if (parsedMaxDevices !== (user.maxDevicesPerProfile ?? 0)) data.maxDevicesPerProfile = parsedMaxDevices;

      if (isSelf && credentialChange) data.currentPassword = currentPwd;
      if (Object.keys(data).length > 0) {
        await updateUser.mutateAsync({ id: user.id, data });
      }

      if (!isAdmin) {
        const origIds = new Set(accessData?.libraryIds ?? []);
        const origMode = accessData?.restricted || origIds.size > 0 ? "selected" : "all";
        const changed =
          libMode !== origMode ||
          (libMode === "selected" && (selectedLibs.size !== origIds.size || [...selectedLibs].some((id) => !origIds.has(id))));
        if (changed) {
          if (libMode === "all") {
            await revokeAccess.mutateAsync({ userId: user.id });
          } else {
            await setAccess.mutateAsync({ userId: user.id, data: { libraryIds: Array.from(selectedLibs) } });
          }
        }
      }

      queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
      queryClient.invalidateQueries({ queryKey: getLibraryAccessServiceGetUserLibraryAccessQueryKey(user.id) });
      toast.success(t("admin.users.userUpdated") || "User updated");
      onClose();
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err as Error, t, t("admin.users.failedUpdateUser")));
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
        {isSelf && (
          <div className="flex flex-col gap-1">
            <Input label={t("auth.settings.account.currentPassword")} type="password" autoComplete="current-password" value={currentPwd} onChange={(e) => setCurrentPwd(e.target.value)} />
            <p className="text-xs text-[var(--text-muted)]">{t("auth.settings.account.currentPasswordHint")}</p>
          </div>
        )}
        <Checkbox label={t("admin.users.admin")} checked={isAdmin} onChange={setIsAdmin} />
        <div className="flex flex-col gap-1">
          <Input
            label={t("admin.users.maxProfiles")}
            type="number"
            min="0"
            value={maxProfiles}
            onChange={(e) => setMaxProfiles(e.target.value)}
          />
          <p className="text-xs text-[var(--text-muted)]">{t("admin.users.maxProfilesHelp")}</p>
        </div>
        <div className="flex flex-col gap-1">
          <Input
            label={t("admin.users.maxDevicesPerProfile")}
            type="number"
            min="0"
            value={maxDevicesPerProfile}
            onChange={(e) => setMaxDevicesPerProfile(e.target.value)}
          />
          <p className="text-xs text-[var(--text-muted)]">{t("admin.users.maxDevicesPerProfileHelp")}</p>
        </div>
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
                {libMode === "all"
                  ? t("admin.users.unrestrictedDesc")
                  : selectedLibs.size === 0
                    ? t("admin.users.noLibrariesDesc")
                    : t("admin.users.restrictedDesc", { count: selectedLibs.size, libraryWord: selectedLibs.size === 1 ? "library" : "libraries" })}
              </p>
              <SegmentedControl
                label={t("admin.users.libraryAccessTitle", { name: user.username })}
                options={[
                  { value: "all", label: t("admin.users.accessAll") },
                  { value: "selected", label: t("admin.users.accessSelected") },
                ]}
                value={libMode}
                onChange={(v) => setLibMode(v as "all" | "selected")}
              />
              {libMode === "all" ? (
                <Badge variant="success">{t("admin.users.unrestricted")}</Badge>
              ) : selectedLibs.size === 0 ? (
                <Badge variant="warning">{t("admin.users.noLibraries")}</Badge>
              ) : null}
              {libMode === "selected" && (
                <div className="flex flex-col gap-2">
                  {libraries.map((lib) => (
                    <div key={lib.id} className="px-3 py-2 rounded-[var(--radius-md)] hover:opacity-80 bg-[var(--surface)]">
                      <Checkbox
                        label={lib.name ?? ""}
                        checked={selectedLibs.has(lib.id ?? "")}
                        onChange={() => handleToggleLib(lib.id ?? "")}
                      />
                    </div>
                  ))}
                </div>
              )}
            </div>
          ) : null
        )}
        <div className="flex flex-col gap-2 border-t border-[var(--border)] pt-4">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium text-[var(--text)]">
              {t("admin.users.profilesTitle", "Profiles")}
            </p>
            <Button
              type="button"
              variant="ghost"
              disabled={!user.id}
              onClick={() => setCreatingProfile(true)}
            >
              {t("auth.profile.manage.newProfile", "New profile")}
            </Button>
          </div>
          {profilesLoading ? (
            <div className="flex justify-center py-2">
              <Spinner size="sm" />
            </div>
          ) : profiles.length === 0 ? (
            <p className="text-xs text-[var(--text-muted)]">
              {t("admin.users.noProfiles", "This user has no profiles yet.")}
            </p>
          ) : (
            <ProfileManagerList userId={user.id ?? ""} profiles={profiles} canSetDefault />
          )}
        </div>
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
          <Button type="submit" loading={saving}>{t("common.save")}</Button>
        </div>
      </form>
      {user.id && (
        <CreateProfileDialog
          open={creatingProfile}
          userId={user.id}
          hasKidsProfile={profiles.some((p) => p.isKids === true)}
          onClose={() => setCreatingProfile(false)}
        />
      )}
    </Modal>
  );
}
