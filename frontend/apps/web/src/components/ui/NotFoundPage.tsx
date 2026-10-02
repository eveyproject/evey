import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { EmptyState } from "@/components/ui/EmptyState";

/**
 * Any address the router does not know: the app's own empty state, themed
 * and translated, with the way home - not the router's bare "Not Found".
 */
export function NotFoundPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--bg)] p-6">
      <EmptyState
        title={t("common.notFound.title")}
        description={t("common.notFound.description")}
        action={{ label: t("common.errorBoundary.goHome"), onClick: () => void navigate({ to: "/" }) }}
      />
    </div>
  );
}
