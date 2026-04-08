import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useUserServiceCreateInviteLink } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";

export function CreateInviteForm({ onSuccess }: { onSuccess: () => void }) {
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
