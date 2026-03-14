import { useState } from "react";
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
        className="relative w-full overflow-hidden transition-all duration-200 ease-out group-hover:scale-105 group-hover:shadow-[0_8px_24px_rgba(0,0,0,0.4)]"
        style={{
          aspectRatio: "2/3",
          borderRadius: "var(--radius-md)",
          boxShadow: "0 2px 8px rgba(0,0,0,0.2)",
        }}
      >
        {collection.posterUrl && !imgError ? (
          <img
            src={collection.posterUrl}
            alt={collection.name}
            loading="lazy"
            className="w-full h-full object-cover transition-opacity duration-300"
            style={{ opacity: imgLoaded ? 1 : 0 }}
            onLoad={() => setImgLoaded(true)}
            onError={() => setImgError(true)}
          />
        ) : null}

        {(!collection.posterUrl || imgError) && (
          <div
            className="absolute inset-0 flex items-center justify-center text-2xl font-bold"
            style={{ backgroundColor: "var(--surface-raised)", color: "var(--text-muted)" }}
          >
            {initial}
          </div>
        )}

        <div
          className="absolute inset-0"
          style={{ background: "linear-gradient(to top, rgba(0,0,0,0.7) 0%, transparent 55%)" }}
        />

        <div
          className="absolute inset-0 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-200"
          style={{ background: "rgba(0,0,0,0.25)" }}
        >
          <svg
            className="w-10 h-10"
            viewBox="0 0 24 24"
            fill="none"
            stroke="rgba(255,255,255,0.9)"
            strokeWidth={1.5}
            aria-hidden="true"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />
          </svg>
        </div>
      </div>

      <div className="mt-2 px-0.5">
        <p className="text-sm font-medium truncate" style={{ color: "var(--text)" }}>
          {collection.name}
        </p>
      </div>
    </Link>
  );
}
