import { type ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "ghost" | "danger" | "overlay" | "overlay-accent";

type Size = "md" | "lg";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  /** "lg" is the media-detail action row, where the surrounding type is larger.
      Icons passed as children are sized at the call site - keep them in step
      (ICON.md, the step the type scale pairs with text-base). */
  size?: Size;
  loading?: boolean;
}

// The action row's box is 42px tall: text-base (16px) at leading-snug (22px)
// plus 10px of padding top and bottom. Height comes from PADDING, never from a
// fixed h-*: a long label ("Ajouter à la liste de lecture") wraps to two lines
// on a narrow screen, and a fixed height would clip it instead of growing.
// ACTION_ICON_BOX below is the icon-only counterpart at the same 42px.
const sizeClasses: Record<Size, string> = {
  md: "gap-2 px-4 py-2 text-sm",
  lg: "gap-2 px-4.5 py-2.5 text-base leading-snug",
};

// Icon-only controls on the action scale: an explicit square, not padding.
// These have no text to wrap, and a stretched flex item is what turned the
// heart and the rating thumbs into 40x48 ovals next to the pill-shaped text
// buttons - the mismatch that made the row read as two different designs.
// Pairs with ICON.xl (24px) inside, leaving a 9px ring.
export const ACTION_ICON_BOX = "w-10.5 h-10.5 shrink-0 flex items-center justify-center";

const variantClasses: Record<Variant, string> = {
  primary: "bg-[var(--accent)] hover:bg-[var(--accent-hover)] text-[var(--filter-active-text)]",
  secondary: "bg-[var(--surface-raised)] hover:bg-[var(--border)] text-[var(--text)]",
  ghost: "bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]",
  danger: "bg-[var(--danger-bg)] hover:bg-[var(--danger-hover)] text-[var(--on-danger)]",
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
        <span className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
      )}
      {children}
    </button>
  );
}
