import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { ICON } from "@/constants/iconSizes";
import { classifyError } from "@/utils/errorClassification";

/**
 * What to show when a read failed, as opposed to when it came back empty.
 * Screens that dropped isError rendered their EMPTY state instead, so a network
 * failure read as "your library is empty" or "this collection has no titles"
 * (audited 2026-08-04). Offline is told apart from a server error because only
 * one of the two is worth retrying right away.
 */
export function FetchErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t } = useTranslation();
  const offline = classifyError(error) === "offline";
  return (
    <div data-testid="fetch-error-state">
      <EmptyState
        icon={<TriangleAlert size={ICON["4xl"]} strokeWidth={1.5} />}
        title={offline ? t("common.offline.title") : t("common.errorBoundary.title")}
        description={offline ? t("common.offline.description") : t("common.errorBoundary.description")}
        action={onRetry ? { label: t("common.retry"), onClick: onRetry } : undefined}
      />
    </div>
  );
}
