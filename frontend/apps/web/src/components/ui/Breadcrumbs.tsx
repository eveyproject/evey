import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

interface BreadcrumbItem {
  label: string;
  to?: string;
}

export function Breadcrumbs({ items, className = "" }: { items: BreadcrumbItem[]; className?: string }) {
  const { t } = useTranslation();
  return (
    <nav aria-label={t("common.aria.breadcrumb")} className={`flex items-center gap-2 text-sm mb-4 text-[var(--text-secondary)] ${className}`}>
      {items.map((item, i) => (
        <span key={i} className="flex items-center gap-2">
          {i > 0 && <span aria-hidden="true">›</span>}
          {item.to ? (
            <Link to={item.to} className="hover:text-[var(--text)] transition-colors-fast">{item.label}</Link>
          ) : (
            <span className="text-[var(--text)]">{item.label}</span>
          )}
        </span>
      ))}
    </nav>
  );
}
