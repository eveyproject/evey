import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { WatchHistory } from "@/features/user/WatchHistory";

export const Route = createLazyFileRoute("/_auth/profile/history")({
  component: HistoryPage,
});

function HistoryPage() {
  const { t } = useTranslation("common");
  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <h1 className="text-2xl font-bold text-[var(--text)]">{t("watchHistory.title")}</h1>
      <WatchHistory showClearButton />
    </div>
  );
}
