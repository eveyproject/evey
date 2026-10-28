import type { V1Library, V1LibraryType } from "@evey/api";

export interface LibraryFormData {
  name: string;
  type: V1LibraryType;
  paths: string[];
  realtimeMonitoring: boolean;
}

// On by default for a new library: without it an added or deleted file waits
// for the next periodic scan, and the server already falls back to that scan
// where a watch cannot work (a network share, say).
export function emptyLibraryForm(): LibraryFormData {
  return { name: "", type: "LIBRARY_TYPE_MOVIES", paths: [""], realtimeMonitoring: true };
}

export function formFromLibrary(lib: V1Library): LibraryFormData {
  return {
    name: lib.name ?? "",
    type: (lib.type as V1LibraryType) ?? "LIBRARY_TYPE_MOVIES",
    paths: lib.paths?.length ? [...lib.paths] : [""],
    realtimeMonitoring: lib.realtimeMonitoring ?? false,
  };
}

function cleanPaths(paths: string[]): string[] {
  return paths.filter((p) => p.trim() !== "");
}

export function libraryCreateBody(form: LibraryFormData) {
  return { name: form.name.trim(), type: form.type, paths: cleanPaths(form.paths), realtimeMonitoring: form.realtimeMonitoring };
}

export function libraryUpdateBody(form: LibraryFormData) {
  return { name: form.name.trim(), paths: cleanPaths(form.paths), realtimeMonitoring: form.realtimeMonitoring };
}

export function libraryFormValid(form: LibraryFormData): boolean {
  return form.name.trim() !== "" && cleanPaths(form.paths).length > 0;
}
