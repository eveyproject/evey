import { createFileRoute, Link } from "@tanstack/react-router";
import { useLibraryServiceListLibraries } from "@evey/api";
import type { V1Library, V1LibraryType } from "@evey/api/src/generated/library.schemas";
import { Spinner } from "@/components/ui/Spinner";
import { Badge } from "@/components/ui/Badge";

export const Route = createFileRoute("/_auth/library/")({
  component: LibraryIndex,
});

function libraryTypeLabel(type?: V1LibraryType): string {
  switch (type) {
    case "LIBRARY_TYPE_MOVIES": return "Movies";
    case "LIBRARY_TYPE_SHOWS": return "Shows";
    case "LIBRARY_TYPE_MUSIC": return "Music";
    case "LIBRARY_TYPE_PHOTOS": return "Photos";
    case "LIBRARY_TYPE_BOOKS": return "Books";
    default: return "Library";
  }
}

function LibraryCard({ library }: { library: V1Library }) {
  const label = libraryTypeLabel(library.type);
  const pathCount = library.paths?.length ?? 0;

  return (
    <Link
      to="/library/$libraryId"
      params={{ libraryId: library.id ?? "" }}
      className="block p-5 transition-colors"
      style={{
        backgroundColor: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-lg)",
      }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLElement).style.backgroundColor = "var(--surface-raised)";
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLElement).style.backgroundColor = "var(--surface)";
      }}
    >
      <div className="flex items-start justify-between gap-2 mb-3">
        <h2 className="text-lg font-semibold truncate" style={{ color: "var(--text)" }}>
          {library.name}
        </h2>
        <Badge variant="info">{label}</Badge>
      </div>
      <p className="text-sm" style={{ color: "var(--text-muted)" }}>
        {pathCount} {pathCount === 1 ? "path" : "paths"}
      </p>
    </Link>
  );
}

function LibraryIndex() {
  const { data, isLoading } = useLibraryServiceListLibraries();
  const libraries = data?.libraries ?? [];

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6" style={{ color: "var(--text)" }}>
        Libraries
      </h1>

      {libraries.length === 0 ? (
        <div
          className="text-center py-16 rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <p className="text-lg mb-2" style={{ color: "var(--text)" }}>
            No libraries configured.
          </p>
          <p style={{ color: "var(--text-muted)" }}>
            Go to{" "}
            <Link to="/admin/libraries" style={{ color: "var(--accent)" }}>
              Admin
            </Link>{" "}
            to add one.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {libraries.map((lib) => (
            <LibraryCard key={lib.id} library={lib} />
          ))}
        </div>
      )}
    </div>
  );
}
