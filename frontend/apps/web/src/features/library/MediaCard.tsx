import { Link } from "@tanstack/react-router";

interface MediaCardItem {
  id?: string;
  title?: string;
  year?: number;
  type?: string;
}

interface MediaCardProps {
  item: MediaCardItem;
  href?: string;
  posterSrc?: string;
  showProgress?: boolean;
  progressPercent?: number;
  showYear?: boolean;
  showType?: boolean;
}

const typeLabel = (t?: string) => {
  if (t === "MEDIA_ITEM_TYPE_MOVIE") return "Film";
  if (t === "MEDIA_ITEM_TYPE_SERIES") return "Serie";
  return "";
};

export function MediaCard({
  item,
  href,
  posterSrc,
  showProgress,
  progressPercent,
  showYear = true,
  showType,
}: MediaCardProps) {
  const poster =
    posterSrc ?? (item.id ? `/api/v1/media/${item.id}/artwork/poster` : null);
  const initial = item.title ? item.title[0].toUpperCase() : "?";
  const pct =
    progressPercent !== undefined
      ? Math.min(100, Math.max(0, Math.round(progressPercent)))
      : undefined;

  const linkTo = href ?? "/media/$mediaId";
  const linkParams = href ? undefined : { mediaId: item.id ?? "" };

  return (
    <Link
      to={linkTo}
      params={linkParams as never}
      className="group block flex-shrink-0 w-40"
    >
      <div
        className="relative w-full overflow-hidden transition-transform duration-200 ease-in-out group-hover:scale-105"
        style={{
          aspectRatio: "2/3",
          borderRadius: "var(--radius-md)",
        }}
      >
        {poster ? (
          <img
            src={poster}
            alt={item.title ?? ""}
            loading="lazy"
            className="w-full h-full object-cover"
            onError={(e) => {
              const t = e.currentTarget;
              t.style.display = "none";
              const fb = t.nextElementSibling as HTMLElement | null;
              if (fb) fb.style.display = "flex";
            }}
          />
        ) : null}
        <div
          className="absolute inset-0 items-center justify-center text-2xl font-bold"
          style={{
            backgroundColor: "var(--surface-raised)",
            color: "var(--text-muted)",
            display: poster ? "none" : "flex",
          }}
        >
          {initial}
        </div>

        {showYear && item.year !== undefined && (
          <span
            className="absolute top-2 right-2 text-xs font-medium px-1.5 py-0.5"
            style={{
              backgroundColor: "rgba(0,0,0,0.7)",
              color: "#fff",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {item.year}
          </span>
        )}

        <div
          className="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity"
          style={{ backgroundColor: "rgba(0,0,0,0.5)" }}
        >
          <div
            className="w-12 h-12 rounded-full flex items-center justify-center"
            style={{ backgroundColor: "var(--accent)" }}
          >
            <svg
              className="w-5 h-5 text-white ml-1"
              fill="currentColor"
              viewBox="0 0 20 20"
            >
              <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
            </svg>
          </div>
        </div>

        {showProgress && pct !== undefined && (
          <div
            className="absolute bottom-0 left-0 right-0 h-1"
            style={{ backgroundColor: "var(--border)" }}
          >
            <div
              className="h-full"
              style={{ backgroundColor: "var(--accent)", width: `${pct}%` }}
            />
          </div>
        )}
      </div>

      <div className="mt-2 px-0.5">
        <p
          className="text-sm font-medium truncate"
          style={{ color: "var(--text)" }}
        >
          {item.title}
        </p>
        {showType && typeLabel(item.type) && (
          <span
            className="text-xs px-1.5 py-0.5 rounded"
            style={{
              backgroundColor: "var(--surface-raised)",
              color: "var(--text-muted)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {typeLabel(item.type)}
          </span>
        )}
      </div>
    </Link>
  );
}
