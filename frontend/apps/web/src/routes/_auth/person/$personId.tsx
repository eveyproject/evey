import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_auth/person/$personId")({
  component: PersonPage,
});

/**
 * Person page — shows name, photo and a note about the person.
 * No dedicated person API endpoint exists yet; cast data comes from
 * V1MediaItem.people when navigating from the media detail page.
 * The personId is the person's id from V1MediaPerson.
 */
function PersonPage() {
  const { personId } = Route.useParams();

  if (!personId) {
    return (
      <div className="text-center py-16" style={{ color: "var(--text-muted)" }}>
        Person not found.
      </div>
    );
  }

  return (
    <div className="max-w-2xl mx-auto py-10 px-4">
      <div className="flex items-center gap-6 mb-8">
        <div
          className="w-24 h-24 rounded-full overflow-hidden flex-shrink-0 flex items-center justify-center"
          style={{ backgroundColor: "var(--surface-raised)" }}
        >
          <img
            src={`/api/v1/media/${personId}/artwork/poster`}
            alt=""
            className="w-full h-full object-cover"
            onError={(e) => {
              const t = e.currentTarget;
              t.style.display = "none";
              const fb = t.nextElementSibling as HTMLElement | null;
              if (fb) fb.style.display = "flex";
            }}
          />
          <span
            className="text-3xl font-bold"
            style={{ color: "var(--text-muted)", display: "none" }}
          >
            ?
          </span>
        </div>
        <div>
          <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
            Person
          </h1>
          <p className="text-sm mt-1" style={{ color: "var(--text-muted)" }}>
            ID: {personId}
          </p>
        </div>
      </div>
      <p className="text-sm" style={{ color: "var(--text-muted)" }}>
        Detailed person profiles are not yet available via the API.
      </p>
    </div>
  );
}
