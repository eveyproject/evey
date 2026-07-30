import { useQuery } from "@tanstack/react-query";
import { useLibraryServiceListLibraries, libraryServiceListGenres } from "@evey/api";
import { mergeGenres } from "../lib/mergeGenres";

// The genres the user's libraries ACTUALLY contain, across all of them.
//
// Screens that filter by genre without a library context (global search, smart
// playlist rules) used to offer a hardcoded English list. Genre filters match
// the stored string exactly and metadata providers return genres in the
// server's language, so on a French library every one of those chips matched
// nothing - a filter that cannot return a result. ListGenres is per-library, so
// this fans out over the libraries the caller can see and merges the answers.
//
// Gated behind `enabled` because it costs one request per library: callers pass
// the "filter panel is open" flag rather than paying for it on every mount.
export function useAllGenres(enabled: boolean) {
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraryIds = (librariesData?.libraries ?? [])
    .map((l) => l.id)
    .filter((id): id is string => !!id);

  return useQuery({
    queryKey: ["all-genres", libraryIds],
    enabled: enabled && libraryIds.length > 0,
    queryFn: async () => {
      const lists = await Promise.all(
        libraryIds.map((id) =>
          // One unreadable library must not blank the whole filter list - a
          // library the caller lost access to answers with an error, and the
          // others still have genres worth offering.
          libraryServiceListGenres(id)
            .then((r) => r.genres ?? [])
            .catch(() => [] as string[]),
        ),
      );
      return mergeGenres(lists);
    },
  });
}
