import { useEffect, useState } from "react";
import { eveyFetch, useLibraryServiceCreateLibrary } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import type { V1LibraryType } from "@evey/api/src/generated/library.schemas";

interface LibraryFormProps {
  onSuccess: () => void;
}

export function LibraryForm({ onSuccess }: LibraryFormProps) {
  const [name, setName] = useState("");
  const [type, setType] = useState<V1LibraryType>("LIBRARY_TYPE_MOVIES");
  const [paths, setPaths] = useState<string[]>([""]);
  const [mediaDirs, setMediaDirs] = useState<string[]>([]);

  useEffect(() => {
    eveyFetch<{ name: string; path: string }[]>({
      url: "/api/v1/admin/fs/browse?path=/media",
      method: "GET",
    })
      .then((dirs) => setMediaDirs(dirs.map((d) => d.path)))
      .catch(() => setMediaDirs([]));
  }, []);

  const createLib = useLibraryServiceCreateLibrary({
    mutation: { onSuccess },
  });

  function addPath() {
    setPaths((prev) => [...prev, ""]);
  }

  function removePath(i: number) {
    setPaths((prev) => prev.filter((_, idx) => idx !== i));
  }

  function updatePath(i: number, val: string) {
    setPaths((prev) => prev.map((p, idx) => (idx === i ? val : p)));
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    createLib.mutate({
      data: {
        name,
        type,
        paths: paths.filter((p) => p.trim() !== ""),
      },
    });
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        label="Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />

      <div className="flex flex-col gap-1">
        <label className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>
          Type
        </label>
        <select
          value={type}
          onChange={(e) => setType(e.target.value as V1LibraryType)}
          className="px-3 py-2 rounded-[var(--radius-md)] text-sm"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        >
          <option value="LIBRARY_TYPE_MOVIES">Movies</option>
          <option value="LIBRARY_TYPE_SHOWS">TV Shows</option>
          <option value="LIBRARY_TYPE_MUSIC">Music</option>
        </select>
      </div>

      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>
          Paths
        </span>
        {paths.map((p, i) => (
          <div key={i} className="flex gap-2">
            <select
              value={p}
              onChange={(e) => updatePath(i, e.target.value)}
              className="flex-1 px-3 py-2 rounded-[var(--radius-md)] text-sm"
              style={{
                backgroundColor: "var(--surface-raised)",
                border: "1px solid var(--border)",
                color: p ? "var(--text)" : "var(--text-muted)",
              }}
            >
              <option value="">— select a directory —</option>
              {mediaDirs.map((d) => (
                <option key={d} value={d}>{d}</option>
              ))}
            </select>
            {paths.length > 1 && (
              <Button type="button" variant="danger" onClick={() => removePath(i)}>
                Remove
              </Button>
            )}
          </div>
        ))}
        <Button type="button" variant="secondary" onClick={addPath} className="self-start">
          + Add Path
        </Button>
      </div>

      <div className="flex justify-end gap-2 pt-2">
        <Button
          type="submit"
          loading={createLib.isPending}
          disabled={!name.trim()}
        >
          Create Library
        </Button>
      </div>
    </form>
  );
}
