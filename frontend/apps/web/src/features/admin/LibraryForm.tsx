import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch, useLibraryServiceCreateLibrary, useLibraryServiceUpdateLibrary, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import type { V1Library, V1LibraryType } from "@evey/api";

interface LibraryFormProps {
  onSuccess: () => void;
  library?: V1Library;
}

export function LibraryForm({ onSuccess, library }: LibraryFormProps) {
  const { t } = useTranslation();
  const isEdit = !!library;
  const [name, setName] = useState(library?.name ?? "");
  const [type, setType] = useState<V1LibraryType>((library?.type as V1LibraryType) ?? "LIBRARY_TYPE_MOVIES");
  const [paths, setPaths] = useState<string[]>(library?.paths?.length ? library.paths : [""]);
  const [mediaDirs, setMediaDirs] = useState<string[]>([]);

  useEffect(() => {
    (async () => {
      try {
        const dirs = await eveyFetch<{ name: string; path: string }[]>({
          url: "/api/v1/admin/fs/browse",
          method: "GET",
        });
        setMediaDirs(dirs.map((d) => d.path));
      } catch {
        setMediaDirs([]);
      }
    })();
  }, []);

  const createLib = useLibraryServiceCreateLibrary({
    mutation: {
      onSuccess,
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
    },
  });

  const updateLib = useLibraryServiceUpdateLibrary({
    mutation: {
      onSuccess,
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
    },
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
    const cleanPaths = paths.filter((p) => p.trim() !== "" && p !== "__custom__");
    if (isEdit) {
      updateLib.mutate({
        id: library.id!,
        data: { name, paths: cleanPaths },
      });
    } else {
      createLib.mutate({
        data: { name, type, paths: cleanPaths },
      });
    }
  }

  const isPending = isEdit ? updateLib.isPending : createLib.isPending;

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        label={t("common.name")}
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />

      {!isEdit && (
        <Select
          label={t("admin.libraries.tableHeaders.type")}
          value={type}
          onChange={(v) => setType(v as V1LibraryType)}
          options={[
            { value: "LIBRARY_TYPE_MOVIES", label: t("admin.libraries.types.movies") },
            { value: "LIBRARY_TYPE_SHOWS", label: t("admin.libraries.tvShows") },
            { value: "LIBRARY_TYPE_MUSIC", label: t("admin.libraries.types.music") },
            { value: "LIBRARY_TYPE_PHOTOS", label: t("admin.libraries.types.photos") },
            { value: "LIBRARY_TYPE_BOOKS", label: t("admin.libraries.types.books") },
          ]}
        />
      )}

      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-[var(--text-muted)]">
          {t("admin.libraries.paths")}
          {mediaDirs.length === 0 && (
            <span className="ml-2 text-xs font-normal">
              {t("admin.libraries.pathsHint")}
            </span>
          )}
        </span>

        {paths.map((p, i) => (
          <div key={i} className="flex gap-2 items-center">
            {mediaDirs.length > 0 && p !== "__custom__" ? (
              <Select
                value={p}
                onChange={(v) => updatePath(i, v)}
                placeholder={t("admin.libraries.selectDirectory")}
                options={[
                  ...mediaDirs.map((d) => ({ value: d, label: d })),
                  { value: "__custom__", label: t("admin.libraries.enterCustomPath") },
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
                {t("common.remove")}
              </Button>
            )}
          </div>
        ))}

        <Button type="button" variant="secondary" onClick={addPath} className="self-start">
          {t("admin.libraries.addPath")}
        </Button>
      </div>

      <div className="flex justify-end gap-2 pt-2">
        <Button
          type="submit"
          loading={isPending}
          disabled={!name.trim() || paths.every((p) => !p.trim() || p === "__custom__")}
        >
          {isEdit ? t("common.save") : t("admin.libraries.createLibrary")}
        </Button>
      </div>
    </form>
  );
}
