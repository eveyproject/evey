import { useRouter } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { ChevronLeft } from "lucide-react";
import { ICON } from "@/constants/iconSizes";

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

  // text-base is the same step as the media action buttons this sits above: at
  // text-xl the page chrome read larger than the primary Play button. The
  // chevron keeps ICON.lg - it is the affordance, and shrinking it to match the
  // label would only make the target harder to hit.
  return (
    <button
      onClick={handleBack}
      className={`flex items-center gap-1 text-base text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast ${className}`}
    >
      <ChevronLeft size={ICON.lg} />
      {t("common.back")}
    </button>
  );
}
