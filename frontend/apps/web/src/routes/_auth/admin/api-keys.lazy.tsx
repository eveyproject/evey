import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { APIKeySettings } from "@/features/user/APIKeySettings";
import { Card } from "@/components/ui/Card";

export const Route = createLazyFileRoute("/_auth/admin/api-keys")({
  component: AdminAPIKeysPage,
});

function AdminAPIKeysPage() {
  const { t } = useTranslation();

  return (
    <div className="px-6 py-8 max-w-4xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.apiKeys.title")}</h1>
      <Card>
        <div className="p-6">
          <APIKeySettings />
        </div>
      </Card>
    </div>
  );
}
