import { type ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "ghost" | "danger";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  loading?: boolean;
}

const variantClasses: Record<Variant, string> = {
  primary: "bg-[var(--accent)] hover:bg-[var(--accent-hover)] text-[var(--filter-active-text)]",
  secondary: "bg-[var(--surface-raised)] hover:bg-[var(--border)] text-[var(--text)]",
  ghost: "bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]",
  danger: "bg-[var(--danger-bg)] hover:bg-[var(--danger-hover)] text-[var(--filter-active-text)]",
};

export function Button({
  variant = "primary",
  loading,
  disabled,
  className = "",
  children,
  ...props
}: ButtonProps) {
  return (
    <button
      {...props}
      disabled={disabled || loading}
      className={`inline-flex items-center justify-center gap-2 px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast-fast disabled:opacity-50 disabled:cursor-not-allowed ${variantClasses[variant]} ${className}`}
    >
      {loading && (
        <span className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
      )}
      {children}
    </button>
  );
}
