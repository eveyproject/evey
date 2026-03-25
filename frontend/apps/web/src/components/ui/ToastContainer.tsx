import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useToastStore, type Toast } from "@evey/store";

const icons: Record<Toast["type"], string> = {
  success: "\u2713",
  error: "\u2715",
  info: "i",
  warning: "!",
};

const typeStyles: Record<Toast["type"], { bg: string; border: string; icon: string }> = {
  success: {
    bg: "bg-[var(--toast-success-bg)]",
    border: "border-[var(--toast-success-border)]",
    icon: "bg-[var(--toast-success-icon)] text-[var(--filter-active-text)]",
  },
  error: {
    bg: "bg-[var(--toast-error-bg)]",
    border: "border-[var(--toast-error-border)]",
    icon: "bg-[var(--toast-error-icon)] text-[var(--filter-active-text)]",
  },
  info: {
    bg: "bg-[var(--toast-info-bg)]",
    border: "border-[var(--toast-info-border)]",
    icon: "bg-[var(--accent)] text-[var(--filter-active-text)]",
  },
  warning: {
    bg: "bg-[var(--toast-warning-bg)]",
    border: "border-[var(--toast-warning-border)]",
    icon: "bg-[var(--toast-warning-icon)] text-[var(--filter-active-text)]",
  },
};

function ToastItem({ toast }: { toast: Toast }) {
  const { t } = useTranslation();
  const removeToast = useToastStore((s) => s.removeToast);
  const ref = useRef<HTMLDivElement>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => {
    const duration = toast.duration ?? 4000;
    timerRef.current = setTimeout(() => {
      ref.current?.classList.add("toast-exit");
      setTimeout(() => removeToast(toast.id), 300);
    }, duration);
    return () => clearTimeout(timerRef.current);
  }, [toast.id, toast.duration, removeToast]);

  const style = typeStyles[toast.type];

  return (
    <div
      ref={ref}
      className={`flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] border shadow-lg backdrop-blur-sm toast-enter ${style.bg} ${style.border}`}
      role="alert"
    >
      <span
        className={`flex-shrink-0 w-5 h-5 flex items-center justify-center rounded-full text-xs font-bold ${style.icon}`}
      >
        {icons[toast.type]}
      </span>
      <span className="flex-1 text-sm text-[var(--text)]">{toast.message}</span>
      <button
        onClick={() => removeToast(toast.id)}
        className="flex-shrink-0 text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast-fast text-lg leading-none"
        aria-label={t("common.aria.dismiss")}
      >
        &times;
      </button>
    </div>
  );
}

export function ToastContainer() {
  const toasts = useToastStore((s) => s.toasts);

  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-overlay flex flex-col gap-2 max-w-sm w-full pointer-events-none" aria-live="polite" aria-atomic="true">
      {toasts.map((t) => (
        <div key={t.id} className="pointer-events-auto">
          <ToastItem toast={t} />
        </div>
      ))}
    </div>
  );
}
