import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { ScrollText } from "lucide-react";
import { PageHeader } from "@/components/ui/PageHeader";
import { EmptyState } from "@/components/ui/EmptyState";
import { LogsExplorer } from "@/features/admin/LogsExplorer";
import { useDemoMode } from "@/hooks/useDemoMode";

export const Route = createLazyFileRoute("/_auth/admin/logs")({
  component: LogsPage,
});

function LogsPage() {
  const { t } = useTranslation();
  // The demo refuses every log route (host logs carry other visitors'
  // addresses), so the page says so instead of mounting an explorer whose
  // every request would come back 403.
  const demo = useDemoMode();
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.logs.title")} subtitle={t("admin.logs.subtitle")} />
      {demo ? (
        <EmptyState icon={<ScrollText size={48} strokeWidth={1.5} />} title={t("errors.demo.locked")} description={t("common.demo.banner")} />
      ) : (
        <LogsExplorer />
      )}
    </div>
  );
}
