import { Link } from "@tanstack/react-router";

interface BreadcrumbItem {
  label: string;
  to?: string;
}

export function Breadcrumbs({ items, className = "" }: { items: BreadcrumbItem[]; className?: string }) {
  return (
    <nav aria-label="breadcrumb" className={`flex items-center gap-2 text-sm mb-4 text-white/90 ${className}`}>
      {items.map((item, i) => (
        <span key={i} className="flex items-center gap-2">
          {i > 0 && <span aria-hidden="true">›</span>}
          {item.to ? (
            <Link to={item.to} className="hover:text-white transition-colors">{item.label}</Link>
          ) : (
            <span className="text-white">{item.label}</span>
          )}
        </span>
      ))}
    </nav>
  );
}
