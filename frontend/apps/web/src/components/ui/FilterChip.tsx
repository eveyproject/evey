import type { ReactNode } from "react";

interface FilterChipProps {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
  size?: "sm" | "md";
  icon?: ReactNode;
  className?: string;
  "data-testid"?: string;
}

export function FilterChip({ active, onClick, children, size = "sm", icon, className = "", "data-testid": testId }: FilterChipProps) {
  const sizeClasses = size === "sm"
    ? "px-2 py-0.5 text-xs"
    : "px-3 py-1.5 text-sm";

  return (
    <button
      type="button"
      onClick={onClick}
      // Selection is otherwise carried by background and border alone, so a
      // screen reader announced every chip identically ("button, Movies")
      // whether or not it was the active filter - and so did a low-vision
      // reader who cannot separate the two backgrounds.
      aria-pressed={active}
      data-testid={testId}
      className={`rounded-full font-medium transition-colors-fast cursor-pointer ${sizeClasses} ${icon ? "flex items-center gap-1.5" : ""} ${className}`}
      style={{
        backgroundColor: active ? "var(--accent)" : "var(--surface-raised)",
        color: active ? "var(--filter-active-text)" : "var(--text-muted)",
        border: `1px solid ${active ? "var(--accent)" : "var(--border)"}`,
      }}
    >
      {icon}
      {children}
    </button>
  );
}
