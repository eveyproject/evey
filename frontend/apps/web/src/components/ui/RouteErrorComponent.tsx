import { useRouter, useNavigate } from "@tanstack/react-router";
import { TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { ErrorComponentProps } from "@tanstack/react-router";

export function RouteErrorComponent({ error, reset }: ErrorComponentProps) {
  const router = useRouter();
  const navigate = useNavigate();
  const { t } = useTranslation();

  const handleRetry = () => {
    reset();
    router.invalidate();
  };

  const handleGoHome = () => {
    void navigate({ to: "/home" });
  };

  return (
    <div className="flex min-h-[50vh] flex-col items-center justify-center gap-6 p-8">
      <div
        className="flex h-16 w-16 items-center justify-center rounded-full"
        style={{ backgroundColor: "var(--error-bg-subtle)" }}
      >
        <TriangleAlert size={32} stroke="var(--color-danger, #ef4444)" strokeWidth={1.5} />
      </div>
      <h1 className="text-2xl font-semibold text-text">
        {t("common.routeError.title")}
      </h1>
      <p className="max-w-md text-center text-sm leading-relaxed text-text-muted">
        {error.message || t("common.routeError.description")}
      </p>
      <div className="mt-2 flex gap-3">
        <button
          onClick={handleRetry}
          aria-label={t("common.aria.retryPage")}
          className="cursor-pointer rounded-lg bg-accent px-6 py-2.5 text-sm font-medium text-[var(--overlay-text)] hover:bg-accent-hover"
        >
          {t("common.routeError.retry")}
        </button>
        <button
          onClick={handleGoHome}
          aria-label={t("common.aria.goToHomePage")}
          className="cursor-pointer rounded-lg border border-[var(--border,#333)] bg-transparent px-6 py-2.5 text-sm font-medium text-text-muted hover:text-text"
        >
          {t("common.routeError.goHome")}
        </button>
      </div>
    </div>
  );
}
