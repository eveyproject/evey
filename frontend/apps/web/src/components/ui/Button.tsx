import { type ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "ghost" | "danger" | "overlay" | "overlay-accent";

type Size = "md" | "lg";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  /** "lg" is the media-detail action row, where the surrounding type is larger.
      Icons passed as children are sized at the call site - keep them in step. */
  size?: Size;
  loading?: boolean;
}

const sizeClasses: Record<Size, string> = {
  md: "gap-2 px-4 py-2 text-sm",
  lg: "gap-2.5 px-5 py-2.5 text-lg",
};

const variantClasses: Record<Variant, string> = {
  primary: "bg-[var(--accent)] hover:bg-[var(--accent-hover)] text-[var(--filter-active-text)]",
  secondary: "bg-[var(--surface-raised)] hover:bg-[var(--border)] text-[var(--text)]",
  ghost: "bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]",
  danger: "bg-[var(--danger-bg)] hover:bg-[var(--danger-hover)] text-[var(--filter-active-text)]",
  overlay: "bg-[var(--overlay-btn-bg)] hover:bg-[var(--overlay-btn-hover-bg)] text-white",
  "overlay-accent": "bg-[var(--overlay-btn-hover-bg)] hover:bg-[var(--overlay-btn-bg)] text-white",
};

export function Button({
  variant = "primary",
  size = "md",
  loading,
  disabled,
  type = "button",
  className = "",
  children,
  ...props
}: ButtonProps) {
  return (
    <button
      {...props}
      type={type}
      disabled={disabled || loading}
      className={`inline-flex items-center justify-center rounded-[var(--radius-md)] font-medium transition-colors-fast disabled:opacity-50 disabled:cursor-not-allowed ${sizeClasses[size]} ${variantClasses[variant]} ${className}`}
    >
      {loading && (
        <span className={`${size === "lg" ? "w-5 h-5" : "w-4 h-4"} border-2 border-current border-t-transparent rounded-full animate-spin`} />
      )}
      {children}
    </button>
  );
}
