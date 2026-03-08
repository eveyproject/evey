import type { V1Artwork } from "@evey/api/src/generated/library.schemas";

interface MediaCardItem {
  id?: string;
  title?: string;
  year?: number;
  type?: string;
  artwork?: V1Artwork[];
}

interface MediaCardProps {
  item: MediaCardItem;
  href?: string;
}

export function MediaCard({ item, href }: MediaCardProps) {
  const poster = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url;
  const initial = item.title ? item.title[0].toUpperCase() : "?";
  const link = href ?? (item.id ? `/media/${item.id}` : "#");

  return (
    <a href={link} className="group relative block w-36" style={{ aspectRatio: "2/3" }}>
      <div
        className="relative w-full h-full overflow-hidden"
        style={{ borderRadius: "var(--radius-md)", transition: "transform 0.2s ease" }}
      >
        {poster ? (
          <img
            src={poster}
            alt={item.title ?? ""}
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
          className="w-full h-full items-center justify-center text-2xl font-bold"
          style={{
            backgroundColor: "var(--surface-raised)",
            color: "var(--text-muted)",
            display: poster ? "none" : "flex",
          }}
        >
          {initial}
        </div>

        {item.year !== undefined && (
          <span
            className="absolute top-2 right-2 text-xs font-medium px-1.5 py-0.5"
            style={{
              backgroundColor: "rgba(0,0,0,0.7)",
              color: "var(--text)",
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
            <svg className="w-5 h-5 text-white ml-1" fill="currentColor" viewBox="0 0 20 20">
              <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
            </svg>
          </div>
        </div>
      </div>

      <div className="mt-2 px-0.5">
        <p className="text-sm font-medium truncate" style={{ color: "var(--text)" }}>
          {item.title}
        </p>
      </div>

      <style>{`
        .group:hover > div:first-child { transform: scale(1.05); }
      `}</style>
    </a>
  );
}
