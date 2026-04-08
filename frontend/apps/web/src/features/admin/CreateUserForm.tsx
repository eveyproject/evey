import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  useUserServiceCreateUser,
  useLibraryServiceListLibraries,
} from "@evey/api";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { Checkbox } from "@/components/ui/Checkbox";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Input } from "@/components/ui/Input";

export function CreateUserForm({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
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

  const createUser = useUserServiceCreateUser({
    mutation: {
      onSuccess: async (data) => {
        const userId = data?.id;
        if (userId && !isAdmin && selectedLibs.size > 0) {
          try {
            await eveyFetch<void>({
              url: `/api/v1/admin/users/${userId}/library-access`,
              method: "PUT",
              data: { libraryIds: Array.from(selectedLibs) },
            });
          } catch {
            toast.error(t("admin.users.failedLibraryAccess"));
          }
        }
        onSuccess();
      },
    },
  });

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
