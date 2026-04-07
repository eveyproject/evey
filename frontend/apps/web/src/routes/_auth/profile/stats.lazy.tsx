import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { UserStats } from "@/features/user/UserStats";

export const Route = createLazyFileRoute("/_auth/profile/stats")({
  component: StatsPage,
});

function StatsPage() {
  const { t } = useTranslation();
  return (
    <div className="px-6 py-8 max-w-4xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("common.userStats.title")}</h1>
      <UserStats />
    </div>
  );
}
