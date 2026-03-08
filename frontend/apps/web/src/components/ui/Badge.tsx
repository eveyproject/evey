import { type ReactNode } from "react";

type BadgeVariant = "default" | "success" | "warning" | "error" | "info";

interface BadgeProps {
  variant?: BadgeVariant;
  children: ReactNode;
  className?: string;
}

const variantClasses: Record<BadgeVariant, string> = {
  default: "bg-[var(--surface-raised)] text-[var(--text-muted)]",
  success: "bg-green-900/40 text-green-400",
  warning: "bg-yellow-900/40 text-yellow-400",
  error: "bg-red-900/40 text-red-400",
  info: "bg-blue-900/40 text-blue-400",
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
