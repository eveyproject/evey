import { useRouter } from "@tanstack/react-router";
import { ChevronLeft } from "lucide-react";

interface BackButtonProps {
  fallback?: string;
  className?: string;
}

export function BackButton({ fallback = "/home", className = "" }: BackButtonProps) {
  const router = useRouter();

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
      className={`flex items-center gap-1 text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast ${className}`}
    >
      <ChevronLeft size={16} />
      Back
    </button>
  );
}
