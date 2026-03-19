import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch, useLibraryServiceCreateLibrary } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import type { V1LibraryType } from "@evey/api/src/generated/library.schemas";

interface LibraryFormProps {
  onSuccess: () => void;
}

export function LibraryForm({ onSuccess }: LibraryFormProps) {
  const { t } = useTranslation("admin");
  const [name, setName] = useState("");
  const [type, setType] = useState<V1LibraryType>("LIBRARY_TYPE_MOVIES");
  const [paths, setPaths] = useState<string[]>([""]);
  const [mediaDirs, setMediaDirs] = useState<string[]>([]);

  useEffect(() => {
    eveyFetch<{ name: string; path: string }[]>({
      url: "/api/v1/admin/fs/browse",
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
        paths: paths.filter((p) => p.trim() !== "" && p !== "__custom__"),
      },
    });
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        label={t("common:name")}
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />

      <Select
        label={t("libraries.tableHeaders.type")}
        value={type}
        onChange={(v) => setType(v as V1LibraryType)}
        options={[
          { value: "LIBRARY_TYPE_MOVIES", label: t("libraries.types.movies") },
          { value: "LIBRARY_TYPE_SHOWS", label: t("libraries.tvShows") },
          { value: "LIBRARY_TYPE_MUSIC", label: t("libraries.types.music") },
        ]}
      />

      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>
          {t("libraries.paths")}
          {mediaDirs.length === 0 && (
            <span className="ml-2 text-xs font-normal">
              {t("libraries.pathsHint")}
            </span>
          )}
        </span>

        {paths.map((p, i) => (
          <div key={i} className="flex gap-2 items-center">
            {mediaDirs.length > 0 && p !== "__custom__" ? (
              <Select
                value={p}
                onChange={(v) => updatePath(i, v)}
                placeholder={t("libraries.selectDirectory")}
                options={[
                  ...mediaDirs.map((d) => ({ value: d, label: d })),
                  { value: "__custom__", label: t("libraries.enterCustomPath") },
                ]}
                className="flex-1"
              />
            ) : (
              <Input
                placeholder="/media/movies"
                value={p === "__custom__" ? "" : p}
                onChange={(e) => updatePath(i, e.target.value)}
                autoFocus={p === "__custom__"}
                className="flex-1"
              />
            )}
            {paths.length > 1 && (
              <Button type="button" variant="danger" onClick={() => removePath(i)}>
                {t("common:remove")}
              </Button>
            )}
          </div>
        ))}

        <Button type="button" variant="secondary" onClick={addPath} className="self-start">
          {t("libraries.addPath")}
        </Button>
      </div>

      <div className="flex justify-end gap-2 pt-2">
        <Button
          type="submit"
          loading={createLib.isPending}
          disabled={!name.trim() || paths.every((p) => !p.trim() || p === "__custom__")}
        >
          {t("libraries.createLibrary")}
        </Button>
      </div>
    </form>
  );
}
