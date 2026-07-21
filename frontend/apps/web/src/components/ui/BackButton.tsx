import { useRouter } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { ChevronLeft } from "lucide-react";

interface BackButtonProps {
  fallback?: string;
  className?: string;
}

export function BackButton({ fallback = "/home", className = "" }: BackButtonProps) {
  const router = useRouter();
  const { t } = useTranslation();

  const handleBack = () => {
    if (window.history.length > 1) {
      router.history.back();
    } else {
      void router.navigate({ to: fallback });
    }
  };

  return (
    <button
      onClick={handleBack}
      className={`flex items-center gap-1 text-xl text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast ${className}`}
    >
      <ChevronLeft size={20} />
      {t("common.back")}
    </button>
  );
}
