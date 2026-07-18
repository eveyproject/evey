import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  useUserServiceCreateUser,
  useLibraryServiceListLibraries,
  useLibraryAccessServiceSetUserLibraryAccess,
} from "@evey/api";
import { toast } from "@evey/store";
import { Checkbox } from "@/components/ui/Checkbox";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Input } from "@/components/ui/Input";

export function CreateUserForm({ onSuccess }: { onSuccess: (accessOk: boolean) => void }) {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  const [maxProfiles, setMaxProfiles] = useState<string>("0");
  const [maxDevicesPerProfile, setMaxDevicesPerProfile] = useState<string>("0");
  const [selectedLibs, setSelectedLibs] = useState<Set<string>>(new Set());

  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];

  const handleToggleLib = useCallback((id: string) => {
    setSelectedLibs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }, []);

  const setAccess = useLibraryAccessServiceSetUserLibraryAccess();

  const createUser = useUserServiceCreateUser({
    mutation: {
      onSuccess: async (data) => {
        const userId = data?.id;
        // The user row is created at this point, but its library grants are a
        // second call. If that fails we must NOT report success - otherwise the
        // admin gets a "user created" toast for a user left with zero access.
        let accessOk = true;
        if (userId && !isAdmin && selectedLibs.size > 0) {
          try {
            await setAccess.mutateAsync({
              userId,
              data: { libraryIds: Array.from(selectedLibs) },
            });
          } catch {
            accessOk = false;
            toast.error(t("admin.users.failedLibraryAccess"));
          }
        }
        onSuccess(accessOk);
      },
      // Without onError a rejected create (dup username, weak password, cap
      // reached) just stops the spinner and shows nothing.
      onError: (err: Error) => toast.error(err?.message || t("admin.users.failedCreateUser")),
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const parsedMaxProfiles = Number.isFinite(Number(maxProfiles)) ? Math.max(0, Math.trunc(Number(maxProfiles))) : 0;
    const parsedMaxDevices = Number.isFinite(Number(maxDevicesPerProfile)) ? Math.max(0, Math.trunc(Number(maxDevicesPerProfile))) : 0;
    createUser.mutate({
      data: {
        username,
        email,
        password,
        isAdmin,
        maxProfiles: parsedMaxProfiles,
        maxDevicesPerProfile: parsedMaxDevices,
      },
    });
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
      {!isAdmin && libraries.length > 0 && (
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
                  label={lib.name ?? lib.id ?? ""}
                  checked={selectedLibs.has(lib.id!)}
                  onChange={() => handleToggleLib(lib.id!)}
                />
              </div>
            ))}
          </div>
          {selectedLibs.size > 0 && (
            <Button variant="ghost" onClick={() => setSelectedLibs(new Set())}>
              {t("admin.users.clearAllUnrestricted")}
            </Button>
          )}
        </div>
      )}
      <div className="flex justify-end pt-2">
        <Button type="submit" loading={createUser.isPending} disabled={!username || !password}>
          {t("admin.users.createUser")}
        </Button>
      </div>
    </form>
  );
}
