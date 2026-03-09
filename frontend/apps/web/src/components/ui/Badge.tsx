import { type ReactNode } from "react";

type BadgeVariant = "default" | "success" | "warning" | "error" | "info";

interface BadgeProps {
  variant?: BadgeVariant;
  children: ReactNode;
  className?: string;
}

const variantClasses: Record<BadgeVariant, string> = {
  default: "bg-[var(--surface-raised)] text-[var(--text-muted)]",
  success: "bg-[var(--badge-success-bg,#065f46)] text-[var(--badge-success-text,#6ee7b7)]",
  warning: "bg-[var(--badge-warning-bg,#713f12)] text-[var(--badge-warning-text,#fde047)]",
  error: "bg-[var(--badge-error-bg,#7f1d1d)] text-[var(--badge-error-text,#fca5a5)]",
  info: "bg-[var(--badge-info-bg,#1e3a5f)] text-[var(--badge-info-text,#93c5fd)]",
};

export function Badge({ variant = "default", children, className = "" }: BadgeProps) {
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${variantClasses[variant]} ${className}`}
    >
      {children}
    </span>
  );
}
