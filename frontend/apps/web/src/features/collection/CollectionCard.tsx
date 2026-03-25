import { useState } from "react";
import { LayoutGrid } from "lucide-react";
import { Link } from "@tanstack/react-router";
import type { Collection } from "./api";

interface CollectionCardProps {
  collection: Collection;
}

export function CollectionCard({ collection }: CollectionCardProps) {
  const [imgLoaded, setImgLoaded] = useState(false);
  const [imgError, setImgError] = useState(false);
  const initial = collection.name[0]?.toUpperCase() ?? "?";

  return (
    <Link
      to="/collections/$collectionId"
      params={{ collectionId: collection.id }}
      className="group block flex-shrink-0 w-32 sm:w-40"
      data-testid="collection-card"
    >
      <div
        className="relative w-full overflow-hidden transition-all duration-200 ease-out group-hover:scale-105 aspect-[2/3] rounded-[var(--radius-md)] shadow-[var(--shadow-sm)]"
      >
        {collection.posterUrl && !imgError ? (
          <img
            src={collection.posterUrl}
            alt={collection.name}
            loading="lazy"
            className="w-full h-full object-cover transition-opacity-fast duration-300"
            style={{ opacity: imgLoaded ? 1 : 0 }}
            onLoad={() => setImgLoaded(true)}
            onError={() => setImgError(true)}
          />
        ) : null}

        {(!collection.posterUrl || imgError) && (
          <div
            className="absolute inset-0 flex items-center justify-center text-2xl font-bold bg-[var(--surface-raised)] text-[var(--text-muted)]"
          >
            {initial}
          </div>
        )}

        <div
          className="absolute inset-0"
          style={{ background: "linear-gradient(to top, var(--overlay-bg-strong) 0%, transparent 55%)" }}
        />

        <div
          className="absolute inset-0 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity-fast duration-200 bg-[var(--overlay-chip-bg)]"
        >
          <LayoutGrid size={40} stroke="var(--overlay-text-bright)" strokeWidth={1.5} aria-hidden="true" />
        </div>
      </div>

      <div className="mt-2 px-0.5">
        <p className="text-sm font-medium truncate text-[var(--text)]">
          {collection.name}
        </p>
      </div>
    </Link>
  );
}
