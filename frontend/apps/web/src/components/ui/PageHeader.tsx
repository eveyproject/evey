import type { ReactNode } from "react";

interface PageHeaderProps {
  title: string;
  subtitle?: string;
  actions?: ReactNode;
}

export function PageHeader({ title, subtitle, actions }: PageHeaderProps) {
  return (
    <div className="flex items-center justify-between gap-4 flex-wrap">
      <div>
        <h1 className="text-2xl font-bold text-[var(--text)]">{title}</h1>
        {subtitle && (
          <p className="text-sm mt-1 text-[var(--text-muted)]">{subtitle}</p>
        )}
      </div>
      {actions}
    </div>
  );
}
