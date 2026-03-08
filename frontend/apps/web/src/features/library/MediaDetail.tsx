import { useState } from "react";
import { Link } from "@tanstack/react-router";
import type { V1MediaItem, V1MediaPerson } from "@evey/api/src/generated/library.schemas";
import {
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  useFeedServiceSimilarItems,
  useLibraryServiceListMediaItems,
  useLibraryServiceGetMediaItem,
  useMetadataServiceSetMetadataOverride,
  useMetadataServiceRefreshMetadata,
} from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Input } from "@/components/ui/Input";
import { StreamList } from "./StreamList";
import { MediaCard } from "./MediaCard";

interface MediaDetailProps {
  item: V1MediaItem;
  onMetadataUpdated?: () => void;
}

interface EditForm {
  title: string;
  year: string;
  overview: string;
  genres: string;
  communityRating: string;
  imdbId: string;
  tmdbId: string;
}

function MetadataEditPanel({
  item,
  onSave,
  onCancel,
}: {
  item: V1MediaItem;
  onSave: () => void;
  onCancel: () => void;
}) {
  const [form, setForm] = useState<EditForm>({
    title: item.title ?? "",
    year: item.year ? String(item.year) : "",
    overview: item.overview ?? "",
    genres: item.genres?.join(", ") ?? "",
    communityRating: item.communityRating ? String(item.communityRating) : "",
    imdbId: item.providerIds?.imdb ?? "",
    tmdbId: item.providerIds?.tmdb ?? "",
  });
  const [refreshStatus, setRefreshStatus] = useState<"idle" | "running" | "done" | "error">("idle");
  const setOverride = useMetadataServiceSetMetadataOverride();
  const refresh = useMetadataServiceRefreshMetadata();

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    if (!item.id) return;
    const fields: Array<[string, string]> = [
      ["title", form.title.trim()],
      ["year", form.year.trim()],
      ["overview", form.overview.trim()],
      ["genres", form.genres.trim()],
      ["community_rating", form.communityRating.trim()],
      ["imdb_id", form.imdbId.trim()],
      ["tmdb_id", form.tmdbId.trim()],
    ];
    for (const [fieldName, customValue] of fields) {
      if (customValue) {
        await setOverride.mutateAsync({ data: { mediaItemId: item.id, fieldName, customValue } });
      }
    }
    onSave();
  }

  async function handleRefresh() {
    if (!item.id) return;
    setRefreshStatus("running");
    try {
      await refresh.mutateAsync({ mediaItemId: item.id });
      setRefreshStatus("done");
      setTimeout(() => { setRefreshStatus("idle"); onSave(); }, 1500);
    } catch {
      setRefreshStatus("error");
    }
  }

  function set(field: keyof EditForm) {
    return (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setForm((prev) => ({ ...prev, [field]: e.target.value }));
  }

  return (
    <form onSubmit={handleSave} className="flex flex-col gap-4 p-4 rounded-[var(--radius-lg)]"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}>
      <Input label="Title" value={form.title} onChange={set("title")} required />
      <Input label="Year" type="number" value={form.year} onChange={set("year")} placeholder="2024" />
      <div className="flex flex-col gap-1">
        <label className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>Overview</label>
        <textarea
          value={form.overview}
          onChange={set("overview")}
          rows={4}
          className="px-3 py-2 rounded-[var(--radius-md)] text-sm resize-none focus:outline-none"
          style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)", color: "var(--text)" }}
        />
      </div>
      <Input label="Genres (comma-separated)" value={form.genres} onChange={set("genres")} placeholder="Action, Drama" />
      <Input label="Rating" type="number" step="0.1" min="0" max="10" value={form.communityRating} onChange={set("communityRating")} placeholder="7.5" />
      <div className="grid grid-cols-2 gap-3">
        <Input label="IMDB ID" value={form.imdbId} onChange={set("imdbId")} placeholder="tt1234567" />
        <Input label="TMDB ID" value={form.tmdbId} onChange={set("tmdbId")} placeholder="12345" />
      </div>
      <div className="flex gap-2 flex-wrap pt-1">
        <Button type="submit" loading={setOverride.isPending}>Save overrides</Button>
        <Button
          type="button"
          variant="secondary"
          loading={refreshStatus === "running"}
          onClick={handleRefresh}
        >
          {refreshStatus === "done" ? "✓ Done" : refreshStatus === "error" ? "Error" : "Refresh metadata"}
        </Button>
        <Button type="button" variant="secondary" onClick={onCancel}>Cancel</Button>
      </div>
      {setOverride.isError && <p className="text-sm" style={{ color: "#ef4444" }}>Failed to save.</p>}
    </form>
  );
}

function formatDuration(ms?: string): string {
  if (!ms) return "";
  const totalMs = parseInt(ms, 10);
  if (isNaN(totalMs)) return "";
  const totalSec = Math.floor(totalMs / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatFileSize(bytes?: string): string {
  if (!bytes) return "";
  const n = parseInt(bytes, 10);
  if (isNaN(n)) return "";
  if (n >= 1_073_741_824) return `${(n / 1_073_741_824).toFixed(1)} GB`;
  if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(0)} MB`;
  return `${(n / 1024).toFixed(0)} KB`;
}

function Stars({ rating }: { rating: number }) {
  const full = Math.round(rating / 2);
  return (
    <span className="flex items-center gap-0.5">
      {Array.from({ length: 5 }, (_, i) => (
        <svg
          key={i}
          className="w-3.5 h-3.5"
          fill={i < full ? "var(--accent)" : "none"}
          stroke="var(--accent)"
          strokeWidth={1.5}
          viewBox="0 0 24 24"
        >
          <path d="M12 2l3.09 6.26L22 9.27l-5 4.87L18.18 21 12 17.77 5.82 21 7 14.14 2 9.27l6.91-1.01L12 2z" />
        </svg>
      ))}
    </span>
  );
}

function CastCard({ person }: { person: V1MediaPerson }) {
  const initial = person.name ? person.name[0].toUpperCase() : "?";
  return (
    <Link to="/person/$personId" params={{ personId: person.id ?? "" }} className="flex-shrink-0 w-28 text-center group">
      <div
        className="w-20 h-20 rounded-full mx-auto overflow-hidden flex items-center justify-center mb-2"
        style={{ backgroundColor: "var(--surface-raised)" }}
      >
        {person.thumbUrl ? (
          <img
            src={person.thumbUrl}
            alt={person.name ?? ""}
            className="w-full h-full object-cover"
            onError={(e) => {
              const t = e.currentTarget;
              t.style.display = "none";
              const fb = t.nextElementSibling as HTMLElement | null;
              if (fb) fb.style.display = "flex";
            }}
          />
        ) : null}
        <span
          className="text-xl font-bold"
          style={{
            color: "var(--text-muted)",
            display: person.thumbUrl ? "none" : "flex",
          }}
        >
          {initial}
        </span>
      </div>
      <p className="text-xs font-medium truncate" style={{ color: "var(--text)" }}>
        {person.name}
      </p>
      {person.role && (
        <p className="text-xs truncate" style={{ color: "var(--text-muted)" }}>
          {person.role}
        </p>
      )}
    </Link>
  );
}

function SeasonRow({ seasonId }: { seasonId: string }) {
  const { data: season } = useLibraryServiceGetMediaItem(seasonId);
  const { data: episodesData } = useLibraryServiceListMediaItems({
    parentId: seasonId,
  });
  const [open, setOpen] = useState(false);
  const episodes = [...(episodesData?.items ?? [])].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));

  const seasonLabel = season?.indexNumber != null
    ? `Season ${season.indexNumber}`
    : (season?.title ?? "Season");

  return (
    <div className="mb-2" style={{ border: "1px solid var(--border)", borderRadius: "var(--radius-md)" }}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center justify-between px-4 py-3 text-left"
        style={{ backgroundColor: "var(--surface)", borderRadius: "var(--radius-md)" }}
      >
        <span className="font-medium" style={{ color: "var(--text)" }}>
          {seasonLabel}
        </span>
        <span className="text-sm" style={{ color: "var(--text-muted)" }}>
          {episodes.length} ep {open ? "▲" : "▼"}
        </span>
      </button>
      {open && (
        <div className="divide-y max-h-96 overflow-y-auto" style={{ borderColor: "var(--border)" }}>
          {episodes.map((ep) => (
            <Link
              key={ep.id}
              to="/player/$mediaId"
              params={{ mediaId: ep.id ?? "" }}
              className="flex items-center gap-3 px-4 py-2 hover:opacity-80 transition-opacity"
              style={{ color: "var(--text)" }}
            >
              <span className="text-sm w-10 flex-shrink-0 font-mono font-medium" style={{ color: "var(--accent)" }}>
                E{ep.indexNumber ?? "?"}
              </span>
              <span className="text-sm flex-1 truncate">{ep.title}</span>
              {ep.durationMs && (
                <span className="text-xs flex-shrink-0" style={{ color: "var(--text-muted)" }}>
                  {formatDuration(ep.durationMs)}
                </span>
              )}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

function SeriesSection({ item }: { item: V1MediaItem }) {
  const { data: seasonsData } = useLibraryServiceListMediaItems({
    parentId: item.id ?? "",
  });
  const seasons = [...(seasonsData?.items ?? [])].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  if (!seasons.length) return null;

  return (
    <div className="mt-10">
      <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
        Seasons
      </h2>
      {seasons.map((s) => s.id ? <SeasonRow key={s.id} seasonId={s.id} /> : null)}
    </div>
  );
}

function SeasonEpisodesSection({ item }: { item: V1MediaItem }) {
  const { data: episodesData } = useLibraryServiceListMediaItems({
    parentId: item.id ?? "",
  });
  const episodes = [...(episodesData?.items ?? [])].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  if (!episodes.length) return null;

  return (
    <div className="mt-10">
      <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
        Episodes
      </h2>
      <div className="divide-y" style={{ border: "1px solid var(--border)", borderRadius: "var(--radius-md)", borderColor: "var(--border)" }}>
        {episodes.map((ep) => (
          <Link
            key={ep.id}
            to="/player/$mediaId"
            params={{ mediaId: ep.id ?? "" }}
            className="flex items-center gap-3 px-4 py-2 hover:opacity-80 transition-opacity"
            style={{ color: "var(--text)" }}
          >
            <span className="text-sm w-10 flex-shrink-0 font-mono font-medium" style={{ color: "var(--accent)" }}>
              E{ep.indexNumber ?? "?"}
            </span>
            <span className="text-sm flex-1 truncate">{ep.title}</span>
            {ep.durationMs && (
              <span className="text-xs flex-shrink-0" style={{ color: "var(--text-muted)" }}>
                {formatDuration(ep.durationMs)}
              </span>
            )}
          </Link>
        ))}
      </div>
    </div>
  );
}

function SimilarSection({ item }: { item: V1MediaItem }) {
  const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
  const firstGenre = item.genres?.[0];
  const hasSimilar = (similarData?.items?.length ?? 0) > 0;
  const { data: byGenreData } = useLibraryServiceListMediaItems(
    { genre: firstGenre, type: item.type as never },
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    { query: { enabled: !hasSimilar && !!firstGenre } as any }
  );

  // Feed items have mediaItemId; derive simple card items from them
  const fromFeed: Array<{ id: string; title?: string }> =
    (similarData?.items ?? [])
      .filter((fi) => !!fi.mediaItemId)
      .map((fi) => ({ id: fi.mediaItemId!, title: fi.title }));

  // Fallback: media items from same genre
  const fromGenre: Array<{ id: string; title?: string; year?: number; type?: string }> =
    (byGenreData?.items ?? []).filter((m) => m.id && m.id !== item.id).map((m) => ({
      id: m.id!,
      title: m.title,
      year: m.year,
      type: m.type,
    }));

  const cards = fromFeed.length ? fromFeed : fromGenre;
  if (!cards.length) return null;

  return (
    <div className="mt-10">
      <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
        Similar Titles
      </h2>
      <div className="flex gap-4 overflow-x-auto pb-2" style={{ scrollbarWidth: "thin" }}>
        {cards.map((c) => (
          <MediaCard key={c.id} item={c} />
        ))}
      </div>
    </div>
  );
}

export function MediaDetail({ item, onMetadataUpdated }: MediaDetailProps) {
  const [watched, setWatched] = useState(false);
  const [fileInfoOpen, setFileInfoOpen] = useState(false);
  const [editing, setEditing] = useState(false);

  const { data: parentItem } = useLibraryServiceGetMediaItem(
    item.parentId ?? "",
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    { query: { enabled: !!item.parentId && item.type === "MEDIA_ITEM_TYPE_SEASON" } } as any,
  );

  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: { onSuccess: () => setWatched(true) },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: { onSuccess: () => setWatched(false) },
  });

  const backdropUrl = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_BACKDROP")?.url
    ?? (item.id ? `/api/v1/media/${item.id}/artwork/backdrop` : undefined);
  const posterUrl = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url
    ?? (item.id ? `/api/v1/media/${item.id}/artwork/poster` : undefined);

  const duration = formatDuration(item.durationMs);
  const actors = item.people?.filter((p) => p.type === "PERSON_TYPE_ACTOR") ?? [];
  const directors = item.people?.filter((p) => p.type === "PERSON_TYPE_DIRECTOR") ?? [];

  return (
    <div>
      {/* Backdrop hero */}
      <div className="relative w-full overflow-hidden -mx-6 -mt-6 mb-0" style={{ height: "420px" }}>
        {backdropUrl ? (
          <img
            src={backdropUrl}
            alt=""
            className="w-full h-full object-cover"
          />
        ) : (
          <div className="w-full h-full" style={{ backgroundColor: "var(--surface)" }} />
        )}
        <div
          className="absolute inset-0"
          style={{ background: "linear-gradient(to top, var(--bg) 0%, rgba(0,0,0,0.3) 60%, transparent 100%)" }}
        />
        {/* Title overlay on backdrop */}
        <div className="absolute bottom-0 left-0 right-0 px-6 pb-6">
          <div className="flex items-center gap-2 mb-2 flex-wrap">
            {item.officialRating && (
              <span
                className="text-xs font-medium px-2 py-0.5 border"
                style={{
                  color: "rgba(255,255,255,0.9)",
                  borderColor: "rgba(255,255,255,0.5)",
                  borderRadius: "var(--radius-sm)",
                }}
              >
                {item.officialRating}
              </span>
            )}
            {duration && (
              <span className="text-sm" style={{ color: "rgba(255,255,255,0.8)" }}>
                {duration}
              </span>
            )}
            {item.communityRating !== undefined && item.communityRating > 0 && (
              <span className="flex items-center gap-1.5">
                <Stars rating={item.communityRating} />
                <span className="text-sm" style={{ color: "rgba(255,255,255,0.8)" }}>
                  {item.communityRating.toFixed(1)}
                </span>
              </span>
            )}
          </div>
          {parentItem && (
            <div className="mb-1">
              <Link
                to="/media/$mediaId"
                params={{ mediaId: item.parentId! }}
                className="text-sm font-medium hover:underline"
                style={{ color: "rgba(255,255,255,0.7)" }}
              >
                {parentItem.title}
              </Link>
              <span className="text-sm mx-1" style={{ color: "rgba(255,255,255,0.5)" }}>›</span>
            </div>
          )}
          <div className="flex items-center gap-3 mb-2">
            <h1 className="text-4xl font-bold text-white drop-shadow-lg">
              {item.title}
              {item.year && (
                <span className="text-2xl font-normal ml-3" style={{ color: "rgba(255,255,255,0.7)" }}>
                  ({item.year})
                </span>
              )}
            </h1>
            {item.id && (
              <button
                onClick={() => setEditing((v) => !v)}
                title="Edit metadata"
                className="flex-shrink-0 p-1.5 rounded-full transition-opacity hover:opacity-80"
                style={{ backgroundColor: "rgba(0,0,0,0.4)", color: "rgba(255,255,255,0.8)" }}
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15.232 5.232l3.536 3.536M9 11l6.586-6.586a2 2 0 112.828 2.828L11.828 13.828a2 2 0 01-1.414.586H8v-2.414a2 2 0 01.586-1.414z" />
                </svg>
              </button>
            )}
          </div>
          {item.genres && item.genres.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {item.genres.map((g) => (
                <span
                  key={g}
                  className="text-xs px-2 py-0.5"
                  style={{
                    backgroundColor: "rgba(255,255,255,0.15)",
                    color: "rgba(255,255,255,0.9)",
                    borderRadius: "var(--radius-sm)",
                  }}
                >
                  {g}
                </span>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Content below backdrop */}
      <div className="flex gap-6 mt-6">
        {/* Poster */}
        <div className="flex-shrink-0 w-48 hidden md:block">
          <div
            className="w-48 overflow-hidden"
            style={{ borderRadius: "var(--radius-md)", aspectRatio: "2/3", backgroundColor: "var(--surface-raised)" }}
          >
            {posterUrl ? (
              <img
                src={posterUrl}
                alt={item.title ?? ""}
                className="w-full h-full object-cover"
              />
            ) : (
              <div className="w-full h-full flex items-center justify-center text-3xl font-bold" style={{ color: "var(--text-muted)" }}>
                {item.title?.[0]?.toUpperCase() ?? "?"}
              </div>
            )}
          </div>
        </div>

        {/* Right: overview + actions */}
        <div className="flex-1 space-y-4 min-w-0">
          {item.tagline && (
            <p className="text-base italic" style={{ color: "var(--text-muted)" }}>
              {item.tagline}
            </p>
          )}
          {item.overview && (
            <p className="text-sm leading-relaxed" style={{ color: "var(--text-muted)" }}>
              {item.overview}
            </p>
          )}

          {directors.length > 0 && (
            <p className="text-sm" style={{ color: "var(--text-muted)" }}>
              <span className="font-medium" style={{ color: "var(--text)" }}>Director: </span>
              {directors.map((d) => d.name).join(", ")}
            </p>
          )}

          {item.studios && item.studios.length > 0 && (
            <p className="text-sm" style={{ color: "var(--text-muted)" }}>
              <span className="font-medium" style={{ color: "var(--text)" }}>Studio: </span>
              {item.studios.join(", ")}
            </p>
          )}

          <div className="flex gap-3 flex-wrap pt-2">
            {item.id && item.type !== "MEDIA_ITEM_TYPE_SERIES" && item.type !== "MEDIA_ITEM_TYPE_SEASON" && (
              <Link to="/player/$mediaId" params={{ mediaId: item.id }}>
                <Button variant="primary">
                  <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
                  </svg>
                  Play
                </Button>
              </Link>
            )}
            {item.id && (
              watched ? (
                <Button
                  variant="secondary"
                  loading={markUnwatched.isPending}
                  onClick={() => markUnwatched.mutate({ mediaItemId: item.id! })}
                >
                  Unmark Watched
                </Button>
              ) : (
                <Button
                  variant="secondary"
                  loading={markWatched.isPending}
                  onClick={() => markWatched.mutate({ mediaItemId: item.id! })}
                >
                  ✓ Mark Watched
                </Button>
              )
            )}
          </div>

          {editing && item.id && (
            <MetadataEditPanel
              item={item}
              onSave={() => { setEditing(false); onMetadataUpdated?.(); }}
              onCancel={() => setEditing(false)}
            />
          )}
        </div>
      </div>

      {/* Cast carousel */}
      {actors.length > 0 && (
        <div className="mt-10">
          <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
            Cast
          </h2>
          <div className="flex gap-4 overflow-x-auto pb-2" style={{ scrollbarWidth: "thin" }}>
            {actors.map((person) => (
              <CastCard key={person.id ?? person.name} person={person} />
            ))}
          </div>
        </div>
      )}

      {/* Seasons (series detail) */}
      {item.type === "MEDIA_ITEM_TYPE_SERIES" && <SeriesSection item={item} />}

      {/* Episodes (season detail) */}
      {item.type === "MEDIA_ITEM_TYPE_SEASON" && <SeasonEpisodesSection item={item} />}

      {/* Similar titles */}
      {item.id && item.type !== "MEDIA_ITEM_TYPE_SEASON" && <SimilarSection item={item} />}

      {/* File info collapsible */}
      {(item.filePath || item.fileSize || item.containerFormat || (item.streams && item.streams.length > 0)) && (
        <div className="mt-10">
          <button
            className="flex items-center gap-2 text-sm font-semibold mb-3"
            style={{ color: "var(--text-muted)" }}
            onClick={() => setFileInfoOpen((v) => !v)}
          >
            <svg
              className="w-4 h-4 transition-transform"
              style={{ transform: fileInfoOpen ? "rotate(90deg)" : "rotate(0deg)" }}
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              viewBox="0 0 24 24"
            >
              <path d="M9 18l6-6-6-6" />
            </svg>
            File Info
          </button>
          {fileInfoOpen && (
            <div className="space-y-2">
              {item.filePath && (
                <div className="text-xs font-mono truncate" style={{ color: "var(--text-muted)" }}>
                  {item.filePath}
                </div>
              )}
              <div className="flex gap-4 flex-wrap">
                {item.fileSize && (
                  <Badge variant="default">{formatFileSize(item.fileSize)}</Badge>
                )}
                {item.containerFormat && (
                  <Badge variant="default">{item.containerFormat.toUpperCase()}</Badge>
                )}
                {item.bitrate && (
                  <Badge variant="default">{Math.round(parseInt(item.bitrate) / 1000)} kbps</Badge>
                )}
                {duration && <Badge variant="default">{duration}</Badge>}
              </div>
              {item.streams && item.streams.length > 0 && (
                <div className="mt-4">
                  <StreamList streams={item.streams} />
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
