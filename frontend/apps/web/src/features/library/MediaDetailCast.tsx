import { useRef } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Link } from "@tanstack/react-router";
import type { V1MediaPerson } from "@evey/api";

function CastCard({ person }: { person: V1MediaPerson }) {
  const initial = person.name ? person.name[0].toUpperCase() : "?";
  const personType = person.type?.replace("PERSON_TYPE_", "") ?? "";
  const roleLabel = personType.charAt(0) + personType.slice(1).toLowerCase();

  return (
    <Link to="/person/$personId" params={{ personId: person.id ?? "" }} className="flex-shrink-0 w-28 text-center group">
      <div
        className="w-20 h-20 rounded-full mx-auto overflow-hidden flex items-center justify-center mb-2 transition-transform-fast group-hover:scale-105 bg-[var(--surface-raised)]"
      >
        {person.thumbUrl ? (
          <img
            src={person.thumbUrl}
            alt={person.name ?? ""}
            className="w-full h-full object-cover"
            loading="lazy"
            onError={(e) => {
              const t = e.currentTarget;
              t.style.display = "none";
              const fb = t.nextElementSibling as HTMLElement | null;
              if (fb) fb.style.display = "flex";
            }}
          />
        ) : null}
        <span
          className="text-xl font-bold text-[var(--text-muted)]"
          style={{ display: person.thumbUrl ? "none" : "flex" }}
        >
          {initial}
        </span>
      </div>
      <p className="text-xs font-medium truncate group-hover:underline text-[var(--text)]">
        {person.name}
      </p>
      {person.role ? (
        <p className="text-xs truncate text-[var(--text-muted)]">{person.role}</p>
      ) : personType && personType !== "ACTOR" ? (
        <p className="text-xs truncate text-[var(--text-muted)]">{roleLabel}</p>
      ) : null}
    </Link>
  );
}

export function CastCarousel({ people, title }: { people: V1MediaPerson[]; title: string }) {
  const scrollRef = useRef<HTMLDivElement>(null);

  function scroll(dir: "left" | "right") {
    if (!scrollRef.current) return;
    const amount = scrollRef.current.clientWidth * 0.6;
    scrollRef.current.scrollBy({ left: dir === "left" ? -amount : amount, behavior: "smooth" });
  }

  if (people.length === 0) return null;

  return (
    <div className="mt-10">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-[var(--text)]">{title}</h2>
        {people.length > 6 && (
          <div className="flex gap-1">
            <button onClick={() => scroll("left")} className="p-1.5 rounded-full hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text-muted)]">
              <ChevronLeft size={16} />
            </button>
            <button onClick={() => scroll("right")} className="p-1.5 rounded-full hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text-muted)]">
              <ChevronRight size={16} />
            </button>
          </div>
        )}
      </div>
      <div ref={scrollRef} className="flex gap-4 overflow-x-auto pb-2" style={{ scrollbarWidth: "thin", scrollBehavior: "smooth" }}>
        {people.map((person) => (
          <CastCard key={person.id ?? person.name} person={person} />
        ))}
      </div>
    </div>
  );
}
