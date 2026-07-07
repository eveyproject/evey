import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/ui/PageHeader";
import { LogsExplorer } from "@/features/admin/LogsExplorer";

export const Route = createLazyFileRoute("/_auth/admin/logs")({
  component: LogsPage,
});

function LogsPage() {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.logs.title")} subtitle={t("admin.logs.subtitle")} />
      <LogsExplorer />
    </div>
  );
}
