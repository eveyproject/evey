import { useRouter } from "@tanstack/react-router";

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
      className={`flex items-center gap-1 text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors ${className}`}
    >
      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
      </svg>
      Back
    </button>
  );
}
