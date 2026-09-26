import { useState } from "react";
import { parseInviteProfileCap } from "@evey/client";
import { useTranslation } from "react-i18next";
import {
  useUserServiceCreateInviteLink,
  useLibraryServiceListLibraries,
} from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { toast } from "@evey/store";

export function CreateInviteForm({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useTranslation();
  const [maxUses, setMaxUses] = useState("");
  const [maxProfiles, setMaxProfiles] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  const [libraryIds, setLibraryIds] = useState<string[]>([]);

  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraries = librariesData?.libraries ?? [];
  const createInvite = useUserServiceCreateInviteLink({
    mutation: {
      onSuccess,
      // Surface rejections (invite quota, bad expiry) instead of failing silently.
      onError: (err: Error) => toast.error(err?.message || t("admin.users.failedCreateInvite")),
    },
  });

  function toggleLibrary(id: string) {
    setLibraryIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    // The datetime-local input yields "YYYY-MM-DDTHH:mm" (local time, no seconds
    // or zone), but expiresAt maps to a protobuf Timestamp whose JSON form is
    // strict RFC3339. Sent raw, protojson rejects it and any expiry-dated invite
    // fails - so convert to an ISO-8601 UTC string first.
    let expiresAtISO: string | undefined;
    if (expiresAt) {
      const d = new Date(expiresAt);
      if (!Number.isNaN(d.getTime())) expiresAtISO = d.toISOString();
    }
    createInvite.mutate({
      data: {
        maxUses: maxUses ? parseInt(maxUses) : undefined,
        maxProfiles: parseInviteProfileCap(maxProfiles),
        expiresAt: expiresAtISO,
        isAdmin,
        libraryIds: isAdmin ? [] : libraryIds,
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
        label={t("admin.users.inviteMaxProfilesLabel")}
        type="number"
        min={1}
        value={maxProfiles}
        onChange={(e) => setMaxProfiles(e.target.value)}
      />
      <Input
        label={t("admin.users.expiresAtLabel")}
        type="datetime-local"
        value={expiresAt}
        onChange={(e) => setExpiresAt(e.target.value)}
      />
      <label className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer">
        <input
          type="checkbox"
          checked={isAdmin}
          onChange={(e) => setIsAdmin(e.target.checked)}
        />
        {t("admin.users.inviteIsAdmin")}
      </label>
      {!isAdmin && libraries.length > 0 && (
        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium text-[var(--text)]">
            {t("admin.users.inviteLibraries")}
          </span>
          <p className="text-xs text-[var(--text-muted)]">
            {t("admin.users.inviteLibrariesHelp")}
          </p>
          <div className="flex flex-col gap-1 max-h-48 overflow-y-auto rounded-[var(--radius-sm)] border border-[var(--border)] p-2 bg-[var(--surface)]">
            {libraries.map((lib) => {
              const id = lib.id ?? "";
              const checked = libraryIds.includes(id);
              return (
                <label
                  key={id}
                  className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer px-2 py-1 rounded hover:bg-[var(--surface-raised)]"
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggleLibrary(id)}
                  />
                  <span className="truncate">{lib.name ?? id}</span>
                </label>
              );
            })}
          </div>
        </div>
      )}
      <div className="flex justify-end pt-2">
        <Button type="submit" loading={createInvite.isPending}>
          {t("admin.users.createInviteLink")}
        </Button>
      </div>
    </form>
  );
}
