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
      url: "/api/v1/admin/fs/browse",
      method: "GET",
      params: { path: "/media" },
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
        paths: paths.filter((p) => p.trim() !== "" && p !== "__custom__"),
      },
    });
  }

  const inputStyle: React.CSSProperties = {
    backgroundColor: "var(--surface-raised)",
    border: "1px solid var(--border)",
    color: "var(--text)",
    borderRadius: "var(--radius-md)",
    padding: "0.5rem 0.75rem",
    fontSize: "0.875rem",
    flex: 1,
    outline: "none",
  };

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
          style={{ ...inputStyle, flex: undefined, width: "100%" }}
        >
          <option value="LIBRARY_TYPE_MOVIES">Movies</option>
          <option value="LIBRARY_TYPE_SHOWS">TV Shows</option>
          <option value="LIBRARY_TYPE_MUSIC">Music</option>
        </select>
      </div>

      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>
          Paths
          {mediaDirs.length === 0 && (
            <span className="ml-2 text-xs font-normal">
              (enter the full path inside the container, e.g. /media/movies)
            </span>
          )}
        </span>

        {paths.map((p, i) => (
          <div key={i} className="flex gap-2 items-center">
            {mediaDirs.length > 0 && p !== "__custom__" ? (
              <select
                value={p}
                onChange={(e) => updatePath(i, e.target.value)}
                style={inputStyle}
              >
                <option value="">— select a directory —</option>
                {mediaDirs.map((d) => (
                  <option key={d} value={d}>{d}</option>
                ))}
                <option value="__custom__">Enter custom path...</option>
              </select>
            ) : (
              <input
                style={inputStyle}
                placeholder="/media/movies"
                value={p === "__custom__" ? "" : p}
                onChange={(e) => updatePath(i, e.target.value)}
                autoFocus={p === "__custom__"}
              />
            )}
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
          disabled={!name.trim() || paths.every((p) => !p.trim() || p === "__custom__")}
        >
          Create Library
        </Button>
      </div>
    </form>
  );
}
